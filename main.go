package main

import (
	"fmt"
	"path/filepath"

	"sftp-gui/internal/client"
	"sftp-gui/internal/profiles"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// ---------- Файловый менеджер ----------
func showFileManager() {
	pathEntry = widget.NewEntry()
	pathEntry.SetText(currentDir)
	pathEntry.OnSubmitted = func(path string) {
		changeDir(path)
	}
	disconnectBtn := widget.NewButton("🔌 Отключиться", func() {
		cli.Close()
		cli = nil
		savedProfiles, _ = profiles.Load()
		profilesList = nil
		returnToConnectScreen()
	})
	upBtn := widget.NewButton("⬆ Вверх", func() {
		parent := filepath.Dir(currentDir)
		if parent != currentDir {
			changeDir(parent)
		}
	})
	refreshBtn := widget.NewButton("⟳ Обновить", func() {
		loadDir(currentDir)
	})
	newFolderBtn := widget.NewButton("📁+", showNewFolderDialog)
	newFileBtn := widget.NewButton("📄+", showNewFileDialog)
	uploadBtn := widget.NewButton("⬆ Загрузить", showUploadDialog)
	topBar := container.NewVBox(
		container.NewHBox(upBtn, refreshBtn, newFolderBtn, newFileBtn, uploadBtn, newThemeToggle(), disconnectBtn),
		pathEntry,
	)

	fileList = widget.NewList(
		func() int { return len(currentItems) },
		func() fyne.CanvasObject {
			return NewClickableLabel(nil, nil, nil)
		},
		func(id int, obj fyne.CanvasObject) {
			clickable := obj.(*ClickableLabel)
			item := currentItems[id]
			clickable.SetIcon(item.IsDir)
			text := fmt.Sprintf("  %s  (размер: %d)  %s", item.Name, item.Size, item.ModTime)
			clickable.Label.SetText(text)
			clickable.SetSelected(selectedIds[id])

			clickable.OnTapped = func() {
				if ctrlHeld {
					if selectedIds[id] {
						delete(selectedIds, id)
					} else {
						selectedIds[id] = true
					}
				} else {
					selectedIds = make(map[int]bool)
					selectedIds[id] = true
				}
				fileList.Refresh()
			}
			clickable.OnDoubleTapped = func() {
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

	deleteSelectedBtn := widget.NewButton("🗑 Удалить выбранные", deleteSelectedItems)

	mainContent := container.NewBorder(
		topBar,
		container.NewHBox(downloadBtn, deleteSelectedBtn),
		nil, nil,
		fileList,
	)

	mainWindow.SetContent(container.NewBorder(
		nil,
		statusLabel,
		nil, nil,
		mainContent,
	))
	mainWindow.Resize(fyne.NewSize(1200, 800))
	mainWindow.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
		if e.Name == fyne.KeyDelete {
			deleteSelectedItems()
		}
	})
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
		fyne.NewMenuItem("🗑 Удалить", func() {
			name := item.Name
			msg := "удалить файл"
			if item.IsDir {
				msg = "удалить папку"
			}
			dialog.ShowConfirm("Подтверждение", fmt.Sprintf("%s «%s»?", msg, name),
				func(ok bool) {
					if !ok {
						return
					}
					go func() {
						var err error
						if item.IsDir {
							err = cli.RemoveDir(fullPath)
						} else {
							err = cli.Remove(fullPath)
						}
						if err != nil {
							fyne.Do(func() {
								dialog.ShowError(fmt.Errorf("ошибка удаления: %v", err), mainWindow)
							})
							return
						}
						fyne.Do(func() {
							loadDir(currentDir)
						})
					}()
				}, mainWindow)
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("📁+ Новая папка", func() {
			showNewFolderDialog()
		}),
		fyne.NewMenuItem("📄+ Новый файл", func() {
			showNewFileDialog()
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("⬆ Загрузить файл", func() {
			showUploadDialog()
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

// ---------- Навигация ----------
func loadDir(path string) {
	items, err := cli.ListDir(path)
	if err != nil {
		dialog.ShowError(fmt.Errorf("чтение директории: %v", err), mainWindow)
		return
	}
	currentItems = items
	currentDir = path
	if pathEntry != nil {
		pathEntry.SetText(path)
	}
	if fileList != nil {
		fileList.Refresh()
	}
	selectedIds = make(map[int]bool)
}

func changeDir(path string) {
	loadDir(path)
}
