package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

var (
	sshClient     *ssh.Client
	sftpClient    *sftp.Client
	currentDir    string
	currentItems  []fileItem
	fileList      *widget.List
	statusLabel   *widget.Label
	pathLabel     *widget.Label
	mainWindow    fyne.Window
	selectedIndex int = -1 // храним выбранный индекс
)

type fileItem struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime string
}

func main() {
	a := app.New()
	w := a.NewWindow("SFTP/SSH client")
	mainWindow = w
	w.Resize(fyne.NewSize(800, 600))

	statusLabel = widget.NewLabel("Готов к подключению")
	connectForm := createConnectForm()
	content := container.NewBorder(connectForm, statusLabel, nil, nil,
		container.NewCenter(widget.NewLabel("Введите параметры и нажмите Подключиться")))
	w.SetContent(content)
	w.ShowAndRun()
}

func createConnectForm() *widget.Form {
	hostEntry := widget.NewEntry()
	hostEntry.SetText("localhost:22")
	userEntry := widget.NewEntry()
	userEntry.SetText("user")
	passEntry := widget.NewPasswordEntry()
	keyPathEntry := widget.NewEntry()
	keyPathEntry.SetText("")

	keyFileBtn := widget.NewButton("Выбрать ключ", func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err == nil && reader != nil {
				keyPathEntry.SetText(reader.URI().Path())
				reader.Close()
			}
		}, mainWindow)
	})

	connectBtn := widget.NewButton("Подключиться", func() {
		go func() {
			statusLabel.SetText("Подключение...")
			err := connectSSHAndSFTP(
				hostEntry.Text,
				userEntry.Text,
				passEntry.Text,
				keyPathEntry.Text,
			)
			if err != nil {
				dialog.ShowError(fmt.Errorf("Ошибка: %v", err), mainWindow)
				statusLabel.SetText("Ошибка подключения")
				return
			}
			statusLabel.SetText("Подключено")
			showFileManager()
		}()
	})

	return &widget.Form{
		Items: []*widget.FormItem{
			widget.NewFormItem("Хост:порт", hostEntry),
			widget.NewFormItem("Пользователь", userEntry),
			widget.NewFormItem("Пароль", passEntry),
			widget.NewFormItem("Ключ (опционально)", container.NewBorder(nil, nil, nil, keyFileBtn, keyPathEntry)),
		},
		OnSubmit:   connectBtn.OnTapped,
		SubmitText: "Подключиться",
	}
}

func connectSSHAndSFTP(host, user, password, keyPath string) error {
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
	client, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return err
	}
	sshClient = client
	sftpClient, err = sftp.NewClient(client)
	if err != nil {
		client.Close()
		return err
	}
	wd, err := sftpClient.Getwd()
	if err != nil {
		wd = "/"
	}
	currentDir = wd
	return nil
}

func showFileManager() {
	pathLabel = widget.NewLabel("Текущая: " + currentDir)
	upBtn := widget.NewButton("⬆ Вверх", func() {
		parent := filepath.Dir(currentDir)
		if parent != currentDir {
			changeDir(parent)
		}
	})
	refreshBtn := widget.NewButton("⟳ Обновить", func() {
		loadDir(currentDir)
	})
	topBar := container.NewHBox(upBtn, refreshBtn, pathLabel)

	fileList = widget.NewList(
		func() int { return len(currentItems) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id int, obj fyne.CanvasObject) {
			item := currentItems[id]
			label := obj.(*widget.Label)
			icon := "📄 "
			if item.IsDir {
				icon = "📁 "
			}
			label.SetText(fmt.Sprintf("%s%s  (размер: %d)  %s", icon, item.Name, item.Size, item.ModTime))
		},
	)
	fileList.OnSelected = func(id int) {
		selectedIndex = id // запоминаем индекс
		item := currentItems[id]
		if item.IsDir {
			changeDir(filepath.Join(currentDir, item.Name))
		}
	}

	downloadBtn := widget.NewButton("⬇ Скачать выбранный файл", func() {
		id := selectedIndex
		if id < 0 || id >= len(currentItems) {
			dialog.ShowInformation("Нет выбора", "Выберите файл", mainWindow)
			return
		}
		item := currentItems[id]
		if item.IsDir {
			dialog.ShowInformation("Папка", "Скачивание папок не поддерживается", mainWindow)
			return
		}
		dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil || writer == nil {
				return
			}
			defer writer.Close()
			srcPath := filepath.Join(currentDir, item.Name)
			remote, err := sftpClient.Open(srcPath)
			if err != nil {
				dialog.ShowError(fmt.Errorf("открытие удалённого файла: %v", err), mainWindow)
				return
			}
			defer remote.Close()
			if _, err := io.Copy(writer, remote); err != nil {
				dialog.ShowError(fmt.Errorf("копирование: %v", err), mainWindow)
				return
			}
			dialog.ShowInformation("Успех", "Файл скачан", mainWindow)
		}, mainWindow)
	})

	mainContent := container.NewBorder(
		topBar,
		container.NewHBox(downloadBtn),
		nil, nil,
		fileList,
	)

	mainWindow.SetContent(container.NewBorder(
		nil,
		statusLabel,
		nil, nil,
		mainContent,
	))
	mainWindow.Resize(fyne.NewSize(900, 700))
	loadDir(currentDir)
}

func loadDir(path string) {
	files, err := sftpClient.ReadDir(path)
	if err != nil {
		dialog.ShowError(fmt.Errorf("чтение директории: %v", err), mainWindow)
		return
	}
	currentItems = make([]fileItem, 0, len(files))
	for _, f := range files {
		currentItems = append(currentItems, fileItem{
			Name:    f.Name(),
			IsDir:   f.IsDir(),
			Size:    f.Size(),
			ModTime: f.ModTime().Format("2006-01-02 15:04:05"),
		})
	}
	currentDir = path
	if pathLabel != nil {
		pathLabel.SetText("Текущая: " + path)
	}
	if fileList != nil {
		fileList.Refresh()
	}
	// Сброс выбранного индекса при смене директории
	selectedIndex = -1
}

func changeDir(path string) {
	loadDir(path)
}
