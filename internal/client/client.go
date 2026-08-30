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
}

func New(host, user, password, keyPath string) (*Client, error) {
	c := &Client{}
	if err := c.Connect(host, user, password, keyPath); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) Connect(host, user, password, keyPath string) error {
	if c.ssh != nil || c.sftp != nil {
		c.Close()
	}

	var authMethods []ssh.AuthMethod
	if keyPath != "" {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			return fmt.Errorf("чтение ключа: %v", err)
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			return fmt.Errorf("парсинг ключа: %v", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	if password != "" {
		authMethods = append(authMethods, ssh.Password(password))
	}
	if len(authMethods) == 0 {
		return fmt.Errorf("нужен пароль или ключ")
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	sshClient, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return err
	}
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		return err
	}
	c.ssh = sshClient
	c.sftp = sftpClient
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
