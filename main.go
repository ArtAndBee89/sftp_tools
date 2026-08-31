package main

import (
	"fmt"
	"os"
	"path/filepath"

	"sftp-gui/internal/client"
	"sftp-gui/internal/profiles"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
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
	darkTheme     bool
	savedProfiles []profiles.Profile
	profilesList  *widget.List
)

// ---------- Основная программа ----------
func main() {
	appInstance = app.New()
	w := appInstance.NewWindow("SFTP/SSH клиент")
	mainWindow = w
	w.Resize(fyne.NewSize(800, 600))

	savedProfiles, _ = profiles.Load()
	statusLabel = widget.NewLabel("Готов к подключению")
	connectForm := createConnectForm()
	themeBtn := newThemeToggle()
	topBar := container.NewHBox(themeBtn, widget.NewLabel("Настройки:"))
	content := container.NewBorder(
		topBar,
		statusLabel,
		nil, nil,
		container.NewBorder(
			connectForm,
			nil, nil, nil,
			newProfilesSection(),
		),
	)
	w.SetContent(content)
	w.ShowAndRun()
}

func newThemeToggle() *widget.Button {
	btn := widget.NewButton("", func() {})
	updateBtn := func() {
		if darkTheme {
			btn.SetText("🌙")
		} else {
			btn.SetText("☀️")
		}
	}
	// определяем текущую тему (системную), чтобы кнопка сразу показывала её
	darkTheme = appInstance.Settings().ThemeVariant() == theme.VariantDark
	updateBtn()
	btn.OnTapped = func() {
		darkTheme = !darkTheme
		if darkTheme {
			appInstance.Settings().SetTheme(theme.DarkTheme())
		} else {
			appInstance.Settings().SetTheme(theme.LightTheme())
		}
		updateBtn()
	}
	return btn
}

// ---------- Профили подключения ----------
func newProfilesSection() fyne.CanvasObject {
	label := widget.NewLabel("Сохранённые профили")
	names := make([]string, 0, len(savedProfiles))
	for _, p := range savedProfiles {
		names = append(names, p.Name)
	}
	selectedProfile := -1

	editBtn := widget.NewButton("✏️ Редактировать", func() {
		if selectedProfile < 0 || selectedProfile >= len(savedProfiles) {
			return
		}
		editProfileWindow(savedProfiles[selectedProfile])
	})
	deleteBtn := widget.NewButton("🗑 Удалить", func() {
		if selectedProfile < 0 || selectedProfile >= len(savedProfiles) {
			return
		}
		name := savedProfiles[selectedProfile].Name
		dialog.ShowConfirm("Удалить профиль", fmt.Sprintf("Удалить профиль «%s»?", name),
			func(ok bool) {
				if !ok {
					return
				}
				if _, err := profiles.Remove(name); err != nil {
					dialog.ShowError(err, mainWindow)
					return
				}
				savedProfiles, _ = profiles.Load()
				returnToConnectScreen()
			}, mainWindow)
	})

	profilesList = widget.NewList(
		func() int { return len(names) },
		func() fyne.CanvasObject { return NewClickableLabel(nil, nil, nil) },
		func(id int, obj fyne.CanvasObject) {
			clickable := obj.(*ClickableLabel)
			clickable.SetIcon(false)
			clickable.Label.SetText("  " + names[id])
			clickable.SetSelected(id == selectedProfile)

			clickable.OnTapped = func() {
				if selectedProfile == id {
					selectedProfile = -1
				} else {
					selectedProfile = id
				}
				profilesList.Refresh()
			}
			clickable.OnDoubleTapped = func() {
				if id < 0 || id >= len(savedProfiles) {
					return
				}
				p := savedProfiles[id]
				connectWithCredentials(p.Host, p.User, p.Password, p.KeyPath)
			}
		},
	)

	btnBar := container.NewHBox(editBtn, deleteBtn)
	return container.NewBorder(
		container.NewVBox(label, btnBar),
		nil, nil, nil,
		profilesList,
	)
}

func connectWithCredentials(host, user, password, keyPath string) {
	go func() {
		fyne.Do(func() {
			statusLabel.SetText("Подключение...")
		})
		c, err := client.New(host, user, password, keyPath)
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
}

// ---------- Окно редактирования профиля ----------
func editProfileWindow(p profiles.Profile) {
	w := appInstance.NewWindow("Редактирование: " + p.Name)
	w.Resize(fyne.NewSize(400, 300))

	hostEntry := widget.NewEntry()
	hostEntry.SetText(p.Host)
	userEntry := widget.NewEntry()
	userEntry.SetText(p.User)
	passEntry := widget.NewPasswordEntry()
	passEntry.SetText(p.Password)
	keyPathEntry := widget.NewEntry()
	keyPathEntry.SetText(p.KeyPath)

	keyFileBtn := widget.NewButton("Выбрать ключ", func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err == nil && reader != nil {
				keyPathEntry.SetText(reader.URI().Path())
				reader.Close()
			}
		}, w)
	})

	saveBtn := widget.NewButton("💾 Сохранить", func() {
		updated := profiles.Profile{
			Name:     p.Name,
			Host:     hostEntry.Text,
			User:     userEntry.Text,
			Password: passEntry.Text,
			KeyPath:  keyPathEntry.Text,
		}
		if _, err := profiles.Add(updated); err != nil {
			dialog.ShowError(fmt.Errorf("ошибка сохранения: %v", err), w)
			return
		}
		savedProfiles, _ = profiles.Load()
		refreshProfilesList()
		dialog.ShowInformation("Сохранено", "Профиль обновлён", w)
		w.Close()
	})

	form := &widget.Form{
		Items: []*widget.FormItem{
			widget.NewFormItem("Хост:порт", hostEntry),
			widget.NewFormItem("Пользователь", userEntry),
			widget.NewFormItem("Пароль", passEntry),
			widget.NewFormItem("Ключ", container.NewBorder(nil, nil, nil, keyFileBtn, keyPathEntry)),
		},
	}
	w.SetContent(container.NewVBox(form, saveBtn))
	w.Show()
}

func createConnectForm() fyne.CanvasObject {
	hostEntry := widget.NewEntry()
	hostEntry.SetText("localhost:22")
	userEntry := widget.NewEntry()
	userEntry.SetText("user")
	passEntry := widget.NewPasswordEntry()
	keyPathEntry := widget.NewEntry()
	profileNameEntry := widget.NewEntry()

	keyFileBtn := widget.NewButton("Выбрать ключ", func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err == nil && reader != nil {
				keyPathEntry.SetText(reader.URI().Path())
				reader.Close()
			}
		}, mainWindow)
	})

	connectBtn := widget.NewButton("Подключиться", func() {
		connectWithCredentials(
			hostEntry.Text,
			userEntry.Text,
			passEntry.Text,
			keyPathEntry.Text,
		)
	})
	connectBtn.Importance = widget.HighImportance

	saveProfileBtn := widget.NewButton("💾 Сохранить профиль", func() {
		name := profileNameEntry.Text
		if name == "" {
			dialog.ShowInformation("Нет имени", "Укажите имя профиля", mainWindow)
			return
		}
		p := profiles.Profile{
			Name:     name,
			Host:     hostEntry.Text,
			User:     userEntry.Text,
			Password: passEntry.Text,
			KeyPath:  keyPathEntry.Text,
		}
		if _, err := profiles.Add(p); err != nil {
			dialog.ShowError(fmt.Errorf("сохранение профиля: %v", err), mainWindow)
			return
		}
		savedProfiles, _ = profiles.Load()
		refreshProfilesList()
		dialog.ShowInformation("Профиль сохранён", "Профиль «"+name+"» добавлен", mainWindow)
	})

	saveProfileBtn.Importance = widget.MediumImportance

	form := &widget.Form{
		Items: []*widget.FormItem{
			widget.NewFormItem("Хост:порт", hostEntry),
			widget.NewFormItem("Пользователь", userEntry),
			widget.NewFormItem("Пароль", passEntry),
			widget.NewFormItem("Ключ (опционально)", container.NewBorder(nil, nil, nil, keyFileBtn, keyPathEntry)),
			widget.NewFormItem("Имя профиля", profileNameEntry),
		},
	}

	btnRow := container.NewHBox(
		connectBtn,
		saveProfileBtn,
	)

	return container.NewVBox(
		form,
		btnRow,
	)
}

func refreshProfilesList() {
	if profilesList == nil {
		return
	}
	names := make([]string, 0, len(savedProfiles))
	for _, p := range savedProfiles {
		names = append(names, p.Name)
	}
	// widget.List нельзя переназначить напрямую, поэтому пересоздаём список через внешнюю переменную.
	// Для простоты обновляем лейблы через Refresh: заново инициализируем содержимое.
	// (Полноценное обновление widget.List требует замены контента; ниже — повторное создание.)
	profilesList.Length = func() int { return len(names) }
	profilesList.UpdateItem = func(id int, obj fyne.CanvasObject) {
		clickable := obj.(*ClickableLabel)
		clickable.SetIcon(false)
		clickable.Label.SetText("  " + names[id])
	}
	profilesList.Refresh()
}

// ---------- Файловый менеджер ----------
func showFileManager() {
	pathLabel = widget.NewLabel("Текущая: " + currentDir)
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
	topBar := container.NewHBox(upBtn, refreshBtn, newThemeToggle(), disconnectBtn, pathLabel)

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
func returnToConnectScreen() {
	statusLabel = widget.NewLabel("Готов к подключению")
	connectForm := createConnectForm()
	themeBtn := newThemeToggle()
	topBar := container.NewHBox(themeBtn, widget.NewLabel("Настройки:"))
	content := container.NewBorder(
		topBar,
		statusLabel,
		nil, nil,
		container.NewBorder(
			connectForm,
			nil, nil, nil,
			newProfilesSection(),
		),
	)
	mainWindow.SetContent(content)
	mainWindow.Resize(fyne.NewSize(800, 600))
}

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
