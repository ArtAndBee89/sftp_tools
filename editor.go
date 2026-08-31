package main

import (
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

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