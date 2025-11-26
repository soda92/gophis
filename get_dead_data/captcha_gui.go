//go:build captcha_gui

package main

import (
	"fmt"
	"os"

	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: captcha_gui <path_to_image>")
		os.Exit(1)
	}
	imagePath := os.Args[1]

	a := app.New()
	w := a.NewWindow("Enter Captcha")

	image := canvas.NewImageFromFile(imagePath)
	image.FillMode = canvas.ImageFillOriginal

	entry := widget.NewEntry()
	entry.SetPlaceHolder("Enter captcha code...")
	entry.OnSubmitted = func(text string) {
		fmt.Print(text) // Print to stdout
		w.Close()       // Close the app
	}

	w.SetContent(container.NewVBox(
		image,
		entry,
	))
	w.SetFixedSize(true)
	w.CenterOnScreen()
	w.ShowAndRun()
}
