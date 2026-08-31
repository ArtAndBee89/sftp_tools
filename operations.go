package main

import (
	"fmt"
	"os"
	"path/filepath"

	"sftp-gui/internal/client"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// ---------- Удаление выбранных элементов ----------
func deleteSelectedItems() {
	if cli == nil || len(selectedIds) == 0 {
		return
	}
	type delItem struct {
		path string
		dir  bool
		name string
	}
	var items []delItem
	for i, it := range currentItems {
		if selectedIds[i] {
			items = append(items, delItem{
				path: filepath.Join(currentDir, it.Name),
				dir:  it.IsDir,
				name: it.Name,
			})
		}
	}
	if len(items) == 0 {
		return
	}
	msg := fmt.Sprintf("Удалить %d выбранных элементов?", len(items))
	dialog.ShowConfirm("Подтверждение", msg,
		func(ok bool) {
			if !ok {
				return
			}
			go func() {
				for _, it := range items {
					var err error
					if it.dir {
						err = cli.RemoveDir(it.path)
					} else {
						err = cli.Remove(it.path)
					}
					if err != nil {
						fyne.Do(func() {
							dialog.ShowError(fmt.Errorf("ошибка удаления «%s»: %v", it.name, err), mainWindow)
						})
						return
					}
				}
				fyne.Do(func() {
					loadDir(currentDir)
				})
			}()
		}, mainWindow)
}

func showNewFolderDialog() {
	entry := widget.NewEntry()
	dialog.ShowCustomConfirm("Новая папка", "Создать", "Отмена", entry,
		func(ok bool) {
			if !ok || entry.Text == "" {
				return
			}
			path := filepath.Join(currentDir, entry.Text)
			go func() {
				if err := cli.Mkdir(path); err != nil {
					fyne.Do(func() {
						dialog.ShowError(fmt.Errorf("ошибка создания папки: %v", err), mainWindow)
					})
					return
				}
				fyne.Do(func() {
					loadDir(currentDir)
				})
			}()
		}, mainWindow)
}

func showNewFileDialog() {
	entry := widget.NewEntry()
	entry.SetText("new_file.txt")
	dialog.ShowCustomConfirm("Новый файл", "Создать", "Отмена", entry,
		func(ok bool) {
			if !ok || entry.Text == "" {
				return
			}
			path := filepath.Join(currentDir, entry.Text)
			go func() {
				if err := cli.CreateEmptyFile(path); err != nil {
					fyne.Do(func() {
						dialog.ShowError(fmt.Errorf("ошибка создания файла: %v", err), mainWindow)
					})
					return
				}
				fyne.Do(func() {
					loadDir(currentDir)
				})
			}()
		}, mainWindow)
}

// ---------- Скачивание одного или нескольких файлов ----------
func downloadFile(remotePath string) {
	dialog.ShowFolderOpen(func(list fyne.ListableURI, err error) {
		if err != nil || list == nil {
			return
		}
		localPath := filepath.Join(list.Path(), filepath.Base(remotePath))
		go func() {
			data, err := cli.ReadFile(remotePath)
			if err != nil {
				fyne.Do(func() {
					dialog.ShowError(err, mainWindow)
				})
				return
			}
			if err := os.WriteFile(localPath, data, 0644); err != nil {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("запись файла: %v", err), mainWindow)
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