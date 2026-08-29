package main

import (
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

var (
	appInstance   fyne.App
	sshClient     *ssh.Client
	sftpClient    *sftp.Client
	currentDir    string
	currentItems  []fileItem
	fileList      *widget.List
	statusLabel   *widget.Label
	pathLabel     *widget.Label
	mainWindow    fyne.Window
	selectedIndex int = -1
)

type fileItem struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime string
}

// ---------- Кастомный виджет с поддержкой левого и правого клика ----------
type ClickableLabel struct {
	widget.BaseWidget
	Label             *widget.Label
	OnTapped          func()
	OnDoubleTapped    func()
	OnTappedSecondary func(fyne.Position)
	Selected          bool
}

func (c *ClickableLabel) SetSelected(selected bool) {
	c.Selected = selected
	c.Refresh()
}

func (c *ClickableLabel) Tapped(*fyne.PointEvent) {
	if c.OnTapped != nil {
		c.OnTapped()
	}
}

func (c *ClickableLabel) DoubleTapped(*fyne.PointEvent) {
	if c.OnDoubleTapped != nil {
		c.OnDoubleTapped()
	}
}

func (c *ClickableLabel) TappedSecondary(ev *fyne.PointEvent) {
	if c.OnTappedSecondary != nil {
		c.OnTappedSecondary(ev.AbsolutePosition)
	}
}

func (c *ClickableLabel) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	return &selectableLabelRenderer{c: c, bg: bg, label: c.Label}
}

type selectableLabelRenderer struct {
	c     *ClickableLabel
	bg    *canvas.Rectangle
	label *widget.Label
}

func (r *selectableLabelRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.label}
}

func (r *selectableLabelRenderer) Layout(s fyne.Size) {
	r.bg.Resize(s)
	r.label.Resize(s)
}

func (r *selectableLabelRenderer) MinSize() fyne.Size {
	return r.label.MinSize()
}

func (r *selectableLabelRenderer) Refresh() {
	if r.c.Selected {
		r.bg.FillColor = color.NRGBA{R: 51, G: 153, B: 255, A: 180}
	} else {
		r.bg.FillColor = color.Transparent
	}
	r.bg.Refresh()
	r.label.Refresh()
}

func (r *selectableLabelRenderer) Destroy() {}

func NewClickableLabel(text string, onTap func(), onDoubleTap func(), onTapSecondary func(fyne.Position)) *ClickableLabel {
	label := widget.NewLabel(text)
	label.Alignment = fyne.TextAlignLeading
	c := &ClickableLabel{Label: label, OnTapped: onTap, OnDoubleTapped: onDoubleTap, OnTappedSecondary: onTapSecondary}
	c.ExtendBaseWidget(c)
	return c
}

// ---------- Основная программа ----------
func main() {
	appInstance = app.New()
	w := appInstance.NewWindow("SFTP/SSH клиент")
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
			// Обновление статуса — в главном потоке
			fyne.Do(func() {
				statusLabel.SetText("Подключение...")
			})
			err := connectSSHAndSFTP(
				hostEntry.Text,
				userEntry.Text,
				passEntry.Text,
				keyPathEntry.Text,
			)
			if err != nil {
				// Показываем диалог и статус в главном потоке
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("Ошибка: %v", err), mainWindow)
					statusLabel.SetText("Ошибка подключения")
				})
				return
			}
			fyne.Do(func() {
				statusLabel.SetText("Подключено")
				showFileManager()
			})
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

	// Список с ClickableLabel
	fileList = widget.NewList(
		func() int { return len(currentItems) },
		func() fyne.CanvasObject {
			return NewClickableLabel("", nil, nil, nil)
		},
		func(id int, obj fyne.CanvasObject) {
			clickable := obj.(*ClickableLabel)
			item := currentItems[id]
			icon := "📄 "
			if item.IsDir {
				icon = "📁 "
			}
			text := fmt.Sprintf("%s%s  (размер: %d)  %s", icon, item.Name, item.Size, item.ModTime)
			clickable.Label.SetText(text)
			clickable.SetSelected(id == selectedIndex)

			clickable.OnTapped = func() {
				if id == selectedIndex {
					selectedIndex = -1
				} else {
					selectedIndex = id
				}
				fileList.Refresh()
			}
			clickable.OnDoubleTapped = func() {
				selectedIndex = id
				fullPath := filepath.Join(currentDir, item.Name)
				if item.IsDir {
					changeDir(fullPath)
				} else {
					openFileEditor(fullPath)
				}
			}
			clickable.OnTappedSecondary = func(pos fyne.Position) {
				selectedIndex = id
				showContextMenu(id, pos)
			}
		},
	)

	downloadBtn := widget.NewButton("⬇ Скачать выбранный", func() {
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
		fullPath := filepath.Join(currentDir, item.Name)
		downloadFile(fullPath, item)
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

// ---------- Контекстное меню ----------
func showContextMenu(id int, pos fyne.Position) {
	item := currentItems[id]
	fullPath := filepath.Join(currentDir, item.Name)

	items := []*fyne.MenuItem{
		fyne.NewMenuItem("📂 Открыть", func() {
			if item.IsDir {
				changeDir(fullPath)
			} else {
				openFileEditor(fullPath)
			}
		}),
		fyne.NewMenuItem("⬇ Скачать", func() {
			if item.IsDir {
				dialog.ShowInformation("Папка", "Скачивание папок не поддерживается", mainWindow)
				return
			}
			downloadFile(fullPath, item)
		}),
		fyne.NewMenuItem("ℹ️ Свойства", func() {
			showFileProperties(fullPath, item)
		}),
	}
	menu := fyne.NewMenu("", items...)
	popup := widget.NewPopUpMenu(menu, mainWindow.Canvas())
	popup.ShowAtPosition(pos)
}

// ---------- Скачивание ----------
func downloadFile(remotePath string, item fileItem) {
	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil || writer == nil {
			return
		}
		defer writer.Close()
		// Выполняем сетевые операции в горутине, чтобы не блокировать UI
		go func() {
			remote, err := sftpClient.Open(remotePath)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("открытие файла: %v", err), mainWindow)
				})
				return
			}
			defer remote.Close()
			if _, err := io.Copy(writer, remote); err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("копирование: %v", err), mainWindow)
				})
				return
			}
			fyne.Do(func() {
				dialog.ShowInformation("Успех", "Файл скачан", mainWindow)
			})
		}()
	}, mainWindow)
}

// ---------- Свойства ----------
func showFileProperties(remotePath string, item fileItem) {
	info := fmt.Sprintf(
		"Имя: %s\nПуть: %s\nТип: %s\nРазмер: %d байт\nИзменён: %s",
		item.Name,
		remotePath,
		map[bool]string{true: "Папка", false: "Файл"}[item.IsDir],
		item.Size,
		item.ModTime,
	)
	dialog.ShowInformation("Свойства файла", info, mainWindow)
}

// ---------- Редактор ----------
func openFileEditor(remotePath string) {
	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		dialog.ShowError(fmt.Errorf("не удалось открыть файл: %v", err), mainWindow)
		return
	}
	defer remoteFile.Close()

	data, err := io.ReadAll(remoteFile)
	if err != nil {
		dialog.ShowError(fmt.Errorf("не удалось прочитать файл: %v", err), mainWindow)
		return
	}

	editWin := appInstance.NewWindow("Редактор: " + filepath.Base(remotePath))
	editWin.Resize(fyne.NewSize(600, 400))

	editor := widget.NewMultiLineEntry()
	editor.SetText(string(data))

	saveBtn := widget.NewButton("💾 Сохранить", func() {
		// Выполняем запись в горутине
		go func() {
			remoteFileWrite, err := sftpClient.Create(remotePath)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("не удалось создать файл для записи: %v", err), mainWindow)
				})
				return
			}
			defer remoteFileWrite.Close()

			_, err = remoteFileWrite.Write([]byte(editor.Text))
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("ошибка записи: %v", err), mainWindow)
				})
				return
			}
			fyne.Do(func() {
				dialog.ShowInformation("Успех", "Файл сохранён", editWin)
			})
		}()
	})

	closeBtn := widget.NewButton("❌ Закрыть", func() {
		editWin.Close()
	})

	btnBar := container.NewHBox(saveBtn, closeBtn)
	content := container.NewBorder(btnBar, nil, nil, nil, editor)
	editWin.SetContent(content)
	editWin.Show()
}

// ---------- Навигация ----------
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
	selectedIndex = -1
}

func changeDir(path string) {
	loadDir(path)
}
