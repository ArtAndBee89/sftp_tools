package main

import (
	"fmt"
	"os"
	"path/filepath"

	"sftp-gui/internal/client"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

var (
	appInstance   fyne.App
	cli           *client.Client
	currentDir    string
	currentItems  []client.Item
	fileList      *widget.List
	statusLabel   *widget.Label
	pathLabel     *widget.Label
	mainWindow    fyne.Window
	selectedIds   = make(map[int]bool)
)

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
			fyne.Do(func() {
				statusLabel.SetText("Подключение...")
			})
			c, err := client.New(
				hostEntry.Text,
				userEntry.Text,
				passEntry.Text,
				keyPathEntry.Text,
			)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("Ошибка: %v", err), mainWindow)
					statusLabel.SetText("Ошибка подключения")
				})
				return
			}
			cli = c
			wd, err := cli.Getwd()
			if err != nil {
				wd = "/"
			}
			currentDir = wd
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

// ---------- Файловый менеджер ----------
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
			clickable.SetSelected(selectedIds[id])

			clickable.OnTapped = func() {
				if selectedIds[id] {
					delete(selectedIds, id)
				} else {
					selectedIds[id] = true
				}
				fileList.Refresh()
			}
			clickable.OnDoubleTapped = func() {
				// при двойном клике открываем этот элемент независимо от множественного выбора
				fullPath := filepath.Join(currentDir, item.Name)
				if item.IsDir {
					changeDir(fullPath)
				} else {
					openFileEditor(fullPath)
				}
			}
			clickable.OnTappedSecondary = func(pos fyne.Position) {
				selectedIds[id] = true
				showContextMenu(id, pos)
			}
		},
	)

	downloadBtn := widget.NewButton("⬇ Скачать выбранные", func() {
		if len(selectedIds) == 0 {
			dialog.ShowInformation("Нет выбора", "Выберите файлы", mainWindow)
			return
		}
		path := currentDir
		var chosen []client.Item
		for i, it := range currentItems {
			if selectedIds[i] {
				if it.IsDir {
					dialog.ShowInformation("Папка", "Скачивание папок не поддерживается", mainWindow)
					return
				}
				chosen = append(chosen, it)
			}
		}
		if len(chosen) == 1 {
			downloadFile(filepath.Join(path, chosen[0].Name))
			return
		}
		downloadMultiple(path, chosen)
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
			downloadFile(fullPath)
		}),
		fyne.NewMenuItem("ℹ️ Свойства", func() {
			info := fmt.Sprintf(
				"Имя: %s\nПуть: %s\nТип: %s\nРазмер: %d байт\nИзменён: %s",
				item.Name, fullPath,
				map[bool]string{true: "Папка", false: "Файл"}[item.IsDir],
				item.Size, item.ModTime,
			)
			dialog.ShowInformation("Свойства файла", info, mainWindow)
		}),
	}
	menu := fyne.NewMenu("", items...)
	popup := widget.NewPopUpMenu(menu, mainWindow.Canvas())
	popup.ShowAtPosition(pos)
}

// ---------- Скачивание ----------
func downloadFile(remotePath string) {
	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil || writer == nil {
			return
		}
		defer writer.Close()
		go func() {
			if err := cli.Download(remotePath, writer); err != nil {
				fyne.Do(func() {
					dialog.ShowError(err, mainWindow)
				})
				return
			}
			fyne.Do(func() {
				dialog.ShowInformation("Успех", "Файл скачан", mainWindow)
			})
		}()
	}, mainWindow)
}

func downloadMultiple(basePath string, items []client.Item) {
	dialog.ShowFolderOpen(func(list fyne.ListableURI, err error) {
		if err != nil || list == nil {
			return
		}
		localDir := list.Path()
		go func() {
			for _, it := range items {
				data, err := cli.ReadFile(filepath.Join(basePath, it.Name))
				if err != nil {
					fyne.Do(func() {
						dialog.ShowError(fmt.Errorf("%s: %v", it.Name, err), mainWindow)
					})
					return
				}
				if err := os.WriteFile(filepath.Join(localDir, it.Name), data, 0644); err != nil {
					fyne.Do(func() {
						dialog.ShowError(fmt.Errorf("%s: %v", it.Name, err), mainWindow)
					})
					return
				}
			}
			fyne.Do(func() {
				dialog.ShowInformation("Успех", fmt.Sprintf("Скачано %d файлов", len(items)), mainWindow)
			})
		}()
	}, mainWindow)
}

// ---------- Редактор ----------
func openFileEditor(remotePath string) {
	data, err := cli.ReadFile(remotePath)
	if err != nil {
		dialog.ShowError(err, mainWindow)
		return
	}

	editWin := appInstance.NewWindow("Редактор: " + filepath.Base(remotePath))
	editWin.Resize(fyne.NewSize(600, 400))

	editor := widget.NewMultiLineEntry()
	editor.SetText(string(data))

	saveBtn := widget.NewButton("💾 Сохранить", func() {
		go func() {
			if err := cli.WriteFile(remotePath, []byte(editor.Text)); err != nil {
				fyne.Do(func() {
					dialog.ShowError(err, mainWindow)
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
	items, err := cli.ListDir(path)
	if err != nil {
		dialog.ShowError(fmt.Errorf("чтение директории: %v", err), mainWindow)
		return
	}
	currentItems = items
	currentDir = path
	if pathLabel != nil {
		pathLabel.SetText("Текущая: " + path)
	}
	if fileList != nil {
		fileList.Refresh()
	}
	selectedIds = make(map[int]bool)
}

func changeDir(path string) {
	loadDir(path)
}
