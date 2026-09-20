package client

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Item struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime string
}

type Client struct {
	ssh  *ssh.Client
	sftp *sftp.Client
	sudo bool

	user      string
	auth      []ssh.AuthMethod
	proxyHost string
	proxyUser string
	proxyAuth []ssh.AuthMethod
}

// SetSudo включает/выключает sudo. Если соединение уже установлено,
// sftp-слой пересоздаётся в новом режиме без разрыва SSH.
func (c *Client) SetSudo(enabled bool) error {
	if c.sudo == enabled {
		return nil
	}
	c.sudo = enabled
	if c.ssh == nil {
		return nil
	}
	if c.sftp != nil {
		c.sftp.Close()
		c.sftp = nil
	}
	return c.initSftp()
}

func (c *Client) IsSudo() bool {
	return c.sudo
}

// initSftp создаёт sftp-слой поверх существующего ssh-соединения,
// учитывая текущий режим sudo.
func (c *Client) initSftp() error {
	var sftpClient *sftp.Client
	var err error
	if c.sudo {
		session, err := c.ssh.NewSession()
		if err != nil {
			return fmt.Errorf("создание сессии: %v", err)
		}
		stdin, err := session.StdinPipe()
		if err != nil {
			session.Close()
			return fmt.Errorf("stdin: %v", err)
		}
		stdout, err := session.StdoutPipe()
		if err != nil {
			session.Close()
			return fmt.Errorf("stdout: %v", err)
		}
		if err := session.Start("sudo /usr/lib/openssh/sftp-server"); err != nil {
			session.Close()
			return fmt.Errorf("запуск sudo sftp: %v", err)
		}
		sftpClient, err = sftp.NewClientPipe(stdout, stdin)
		if err != nil {
			session.Close()
			return fmt.Errorf("sftp через sudo: %v", err)
		}
	} else {
		sftpClient, err = sftp.NewClient(c.ssh)
		if err != nil {
			return fmt.Errorf("sftp: %v", err)
		}
	}
	c.sftp = sftpClient
	return nil
}

func authMethods(password, keyPath string) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if keyPath != "" {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("чтение ключа: %v", err)
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			return nil, fmt.Errorf("парсинг ключа: %v", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if password != "" {
		methods = append(methods, ssh.Password(password))
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("нужен пароль или ключ")
	}
	return methods, nil
}

func sshConfig(user string, auth []ssh.AuthMethod) *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User:            user,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
}

func New(host, user, password, keyPath string) (*Client, error) {
	c := &Client{}
	if err := c.Connect(host, user, password, keyPath, "", "", "", ""); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) Connect(host, user, password, keyPath, proxyHost, proxyUser, proxyPassword, proxyKeyPath string) error {
	if c.ssh != nil || c.sftp != nil {
		c.Close()
	}

	auth, err := authMethods(password, keyPath)
	if err != nil {
		return err
	}
	c.user = user
	c.auth = auth
	c.proxyHost = proxyHost
	c.proxyUser = proxyUser
	if proxyHost != "" {
		c.proxyAuth, err = authMethods(proxyPassword, proxyKeyPath)
		if err != nil {
			return fmt.Errorf("прокси: %v", err)
		}
	}

	var sshClient *ssh.Client
	if proxyHost != "" {
		proxyConn, err := ssh.Dial("tcp", proxyHost, sshConfig(proxyUser, c.proxyAuth))
		if err != nil {
			return fmt.Errorf("подключение к прокси: %v", err)
		}
		conn, err := proxyConn.Dial("tcp", host)
		if err != nil {
			proxyConn.Close()
			return fmt.Errorf("туннель через прокси: %v", err)
		}
		nc, ch, req, err := ssh.NewClientConn(conn, host, sshConfig(user, auth))
		if err != nil {
			conn.Close()
			proxyConn.Close()
			return fmt.Errorf("рукопожатие через прокси: %v", err)
		}
		sshClient = ssh.NewClient(nc, ch, req)
	} else {
		sshClient, err = ssh.Dial("tcp", host, sshConfig(user, auth))
		if err != nil {
			return err
		}
	}

	c.ssh = sshClient
	if err := c.initSftp(); err != nil {
		c.ssh.Close()
		return err
	}
	return nil
}

func (c *Client) Close() {
	if c.sftp != nil {
		c.sftp.Close()
		c.sftp = nil
	}
	if c.ssh != nil {
		c.ssh.Close()
		c.ssh = nil
	}
}

func (c *Client) Getwd() (string, error) {
	return c.sftp.Getwd()
}

func (c *Client) ListDir(path string) ([]Item, error) {
	files, err := c.sftp.ReadDir(path)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(files))
	for _, f := range files {
		items = append(items, Item{
			Name:    f.Name(),
			IsDir:   f.IsDir(),
			Size:    f.Size(),
			ModTime: f.ModTime().Format("2006-01-02 15:04:05"),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir // папки первыми
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}

func (c *Client) Download(remotePath string, dst io.Writer) error {
	remote, err := c.sftp.Open(remotePath)
	if err != nil {
		return fmt.Errorf("открытие файла: %v", err)
	}
	defer remote.Close()
	if _, err := io.Copy(dst, remote); err != nil {
		return fmt.Errorf("копирование: %v", err)
	}
	return nil
}

func (c *Client) ReadFile(remotePath string) ([]byte, error) {
	remote, err := c.sftp.Open(remotePath)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть файл: %v", err)
	}
	defer remote.Close()
	data, err := io.ReadAll(remote)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать файл: %v", err)
	}
	return data, nil
}

func (c *Client) WriteFile(remotePath string, data []byte) error {
	remote, err := c.sftp.Create(remotePath)
	if err != nil {
		return fmt.Errorf("не удалось создать файл для записи: %v", err)
	}
	defer remote.Close()
	if _, err := remote.Write(data); err != nil {
		return fmt.Errorf("ошибка записи: %v", err)
	}
	return nil
}

func (c *Client) Mkdir(remotePath string) error {
	return c.sftp.Mkdir(remotePath)
}

func (c *Client) CreateEmptyFile(remotePath string) error {
	remote, err := c.sftp.Create(remotePath)
	if err != nil {
		return err
	}
	return remote.Close()
}

func (c *Client) Remove(remotePath string) error {
	return c.sftp.Remove(remotePath)
}

func (c *Client) RemoveDir(remotePath string) error {
	return c.sftp.RemoveDirectory(remotePath)
}

func (c *Client) Upload(localPath, remotePath string) error {
	local, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("открытие локального файла: %v", err)
	}
	defer local.Close()
	remote, err := c.sftp.Create(remotePath)
	if err != nil {
		return fmt.Errorf("создание удалённого файла: %v", err)
	}
	defer remote.Close()
	if _, err := io.Copy(remote, local); err != nil {
		return fmt.Errorf("копирование на сервер: %v", err)
	}
	return nil
}
