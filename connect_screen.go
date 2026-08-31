package main

import (
	"fmt"

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
	pathEntry     *widget.Entry
	mainWindow    fyne.Window
	selectedIds   = make(map[int]bool)
	darkTheme     bool
	savedProfiles []profiles.Profile
	profilesList  *widget.List
	ctrlHeld      bool
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

// ---------- Соединения подключения ----------
func newProfilesSection() fyne.CanvasObject {
	label := widget.NewLabel("Сохранённые соединения")
	names := make([]string, 0, len(savedProfiles))
	for _, p := range savedProfiles {
		names = append(names, p.Name)
	}
	selectedProfile := -1

	connectToBtn := widget.NewButton("🔌 Подключиться к соединению", func() {
		if selectedProfile < 0 || selectedProfile >= len(savedProfiles) {
			return
		}
		p := savedProfiles[selectedProfile]
		connectWithCredentials(p.Host, p.User, p.Password, p.KeyPath, p.ProxyHost, p.ProxyUser, p.ProxyPassword, p.ProxyKeyPath)
	})
	connectToBtn.Importance = widget.HighImportance
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
		dialog.ShowConfirm("Удалить соединение", fmt.Sprintf("Удалить соединение «%s»?", name),
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
				connectWithCredentials(p.Host, p.User, p.Password, p.KeyPath, p.ProxyHost, p.ProxyUser, p.ProxyPassword, p.ProxyKeyPath)
			}
		},
	)

	btnBar := container.NewHBox(connectToBtn, editBtn, deleteBtn)
	return container.NewBorder(
		container.NewVBox(label, btnBar),
		nil, nil, nil,
		profilesList,
	)
}

func connectWithCredentials(host, user, password, keyPath, proxyHost, proxyUser, proxyPassword, proxyKeyPath string) {
	go func() {
		fyne.Do(func() {
			statusLabel.SetText("Подключение...")
		})
		c := &client.Client{}
		err := c.Connect(host, user, password, keyPath, proxyHost, proxyUser, proxyPassword, proxyKeyPath)
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

// ---------- Окно редактирования соединения ----------
func editProfileWindow(p profiles.Profile) {
	w := appInstance.NewWindow("Редактирование: " + p.Name)
	w.Resize(fyne.NewSize(450, 400))

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

	proxyHostEntry := widget.NewEntry()
	proxyHostEntry.SetText(p.ProxyHost)
	proxyUserEntry := widget.NewEntry()
	proxyUserEntry.SetText(p.ProxyUser)
	proxyPassEntry := widget.NewPasswordEntry()
	proxyPassEntry.SetText(p.ProxyPassword)
	proxyKeyEntry := widget.NewEntry()
	proxyKeyEntry.SetText(p.ProxyKeyPath)

	proxyKeyBtn := widget.NewButton("Выбрать ключ", func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err == nil && reader != nil {
				proxyKeyEntry.SetText(reader.URI().Path())
				reader.Close()
			}
		}, w)
	})

	saveBtn := widget.NewButton("💾 Сохранить", func() {
		updated := profiles.Profile{
			Name:          p.Name,
			Host:          hostEntry.Text,
			User:          userEntry.Text,
			Password:      passEntry.Text,
			KeyPath:       keyPathEntry.Text,
			ProxyHost:     proxyHostEntry.Text,
			ProxyUser:     proxyUserEntry.Text,
			ProxyPassword: proxyPassEntry.Text,
			ProxyKeyPath:  proxyKeyEntry.Text,
		}
		if _, err := profiles.Add(updated); err != nil {
			dialog.ShowError(fmt.Errorf("ошибка сохранения: %v", err), w)
			return
		}
		savedProfiles, _ = profiles.Load()
		refreshProfilesList()
		dialog.ShowInformation("Сохранено", "Соединение обновлено", w)
		w.Close()
	})

	form := &widget.Form{
		Items: []*widget.FormItem{
			widget.NewFormItem("Хост:порт", hostEntry),
			widget.NewFormItem("Пользователь", userEntry),
			widget.NewFormItem("Пароль", passEntry),
			widget.NewFormItem("Ключ", container.NewBorder(nil, nil, nil, keyFileBtn, keyPathEntry)),
			widget.NewFormItem("", widget.NewLabel("SSH-прокси (опционально)")),
			widget.NewFormItem("Хост:порт прокси", proxyHostEntry),
			widget.NewFormItem("Пользователь прокси", proxyUserEntry),
			widget.NewFormItem("Пароль прокси", proxyPassEntry),
			widget.NewFormItem("Ключ прокси", container.NewBorder(nil, nil, nil, proxyKeyBtn, proxyKeyEntry)),
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

	proxyHostEntry := widget.NewEntry()
	proxyUserEntry := widget.NewEntry()
	proxyPassEntry := widget.NewPasswordEntry()
	proxyKeyEntry := widget.NewEntry()
	proxyKeyBtn := widget.NewButton("Выбрать ключ", func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err == nil && reader != nil {
				proxyKeyEntry.SetText(reader.URI().Path())
				reader.Close()
			}
		}, mainWindow)
	})

	connectBtn := widget.NewButton("Подключиться", func() {
		connectWithCredentials(
			hostEntry.Text, userEntry.Text, passEntry.Text, keyPathEntry.Text,
			proxyHostEntry.Text, proxyUserEntry.Text, proxyPassEntry.Text, proxyKeyEntry.Text,
		)
	})
	connectBtn.Importance = widget.HighImportance

	saveProfileBtn := widget.NewButton("💾 Сохранить соединение", func() {
		name := profileNameEntry.Text
		if name == "" {
			dialog.ShowInformation("Нет имени", "Укажите имя соединения", mainWindow)
			return
		}
		p := profiles.Profile{
			Name:          name,
			Host:          hostEntry.Text,
			User:          userEntry.Text,
			Password:      passEntry.Text,
			KeyPath:       keyPathEntry.Text,
			ProxyHost:     proxyHostEntry.Text,
			ProxyUser:     proxyUserEntry.Text,
			ProxyPassword: proxyPassEntry.Text,
			ProxyKeyPath:  proxyKeyEntry.Text,
		}
		if _, err := profiles.Add(p); err != nil {
			dialog.ShowError(fmt.Errorf("сохранение соединения: %v", err), mainWindow)
			return
		}
		savedProfiles, _ = profiles.Load()
		refreshProfilesList()
		dialog.ShowInformation("Соединение сохранено", "Соединение «"+name+"» добавлено", mainWindow)
	})

	saveProfileBtn.Importance = widget.MediumImportance

	form := &widget.Form{
		Items: []*widget.FormItem{
			widget.NewFormItem("Хост:порт", hostEntry),
			widget.NewFormItem("Пользователь", userEntry),
			widget.NewFormItem("Пароль", passEntry),
			widget.NewFormItem("Ключ (опционально)", container.NewBorder(nil, nil, nil, keyFileBtn, keyPathEntry)),
			widget.NewFormItem("", widget.NewLabel("SSH-прокси (опционально)")),
			widget.NewFormItem("Хост:порт прокси", proxyHostEntry),
			widget.NewFormItem("Пользователь прокси", proxyUserEntry),
			widget.NewFormItem("Пароль прокси", proxyPassEntry),
			widget.NewFormItem("Ключ прокси", container.NewBorder(nil, nil, nil, proxyKeyBtn, proxyKeyEntry)),
			widget.NewFormItem("Имя соединения", profileNameEntry),
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
	profilesList.Length = func() int { return len(names) }
	profilesList.UpdateItem = func(id int, obj fyne.CanvasObject) {
		clickable := obj.(*ClickableLabel)
		clickable.SetIcon(false)
		clickable.Label.SetText("  " + names[id])
	}
	profilesList.Refresh()
}

// ---------- Возврат на экран подключения ----------
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