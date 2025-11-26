//go:build config_gui

package main

import (
	"log"
	"os"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/BurntSushi/toml"
)

const configPath = "credentials.toml"

func main() {
	a := app.New()
	w := a.NewWindow("Credentials and URL Mappings Editor")

	// --- Load existing config ---
	creds, err := LoadCredentialsConfig(configPath)
	if err != nil {
		log.Printf("Warning: could not load %s: %v. Starting new config.", configPath, err)
		creds = &CredentialsConfig{
			Credentials: Credentials{},
			URLMappings: make(map[string]string),
		}
	}
	if creds.URLMappings == nil {
		creds.URLMappings = make(map[string]string)
	}

	// --- Basic Credentials Form ---
	urlEntry := widget.NewEntry()
	urlEntry.SetPlaceHolder("Internal Login Page URL")
	urlEntry.SetText(creds.Credentials.URL)

	usernameEntry := widget.NewEntry()
	usernameEntry.SetPlaceHolder("Username")
	usernameEntry.SetText(creds.Credentials.Username)

	passwordEntry := widget.NewPasswordEntry()
	passwordEntry.SetPlaceHolder("Password")
	passwordEntry.SetText(creds.Credentials.Password)

	credsForm := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Internal URL", Widget: urlEntry},
			{Text: "Username", Widget: usernameEntry},
			{Text: "Password", Widget: passwordEntry},
		},
	}

	// --- URL Mappings UI ---
	mappingsData := binding.NewStringList()
	var mappingKeys []string
	for k, v := range creds.URLMappings {
		mappingKeys = append(mappingKeys, k)
		mappingsData.Append(fmt.Sprintf("%s -> %s", k, v))
	}

	mappingList := widget.NewListWithData(
		mappingsData,
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i binding.DataItem, o fyne.CanvasObject) {
			item, _ := i.(binding.String).Get()
			o.(*widget.Label).SetText(item)
		},
	)

	var selectedMappingIndex = -1
	mappingList.OnSelected = func(id widget.ListItemID) {
		selectedMappingIndex = id
	}

	internalURLEntry := widget.NewEntry()
	internalURLEntry.SetPlaceHolder("Internal Base URL (e.g., http://10.0.0.1)")
	publicURLEntry := widget.NewEntry()
	publicURLEntry.SetPlaceHolder("Public/FRP URL (e.g., http://frp.example.com:8080)")

	addMappingBtn := widget.NewButton("Add Mapping", func() {
		internal := internalURLEntry.Text
		public := publicURLEntry.Text
		if internal == "" || public == "" {
			return // Do nothing if fields are empty
		}
		creds.URLMappings[internal] = public
		mappingsData.Append(fmt.Sprintf("%s -> %s", internal, public))
		mappingKeys = append(mappingKeys, internal)
		internalURLEntry.SetText("")
		publicURLEntry.SetText("")
	})

	removeMappingBtn := widget.NewButton("Remove Selected", func() {
		if selectedMappingIndex < 0 || selectedMappingIndex >= len(mappingKeys) {
			return
		}
		keyToRemove := mappingKeys[selectedMappingIndex]
		delete(creds.URLMappings, keyToRemove)
		
		// Rebuild UI list
		mappingsData.Set([]string{})
		mappingKeys = []string{}
		for k, v := range creds.URLMappings {
			mappingKeys = append(mappingKeys, k)
			mappingsData.Append(fmt.Sprintf("%s -> %s", k, v))
		}
		selectedMappingIndex = -1
	})

	mappingsContainer := container.NewBorder(
		container.NewVBox(widget.NewLabel("URL Mappings"), internalURLEntry, publicURLEntry, container.NewGridWithColumns(2, addMappingBtn, removeMappingBtn)),
		nil, nil, nil,
		mappingList,
	)

	// --- Save Button ---
	saveBtn := widget.NewButton("Save All Changes", func() {
		// Update credentials from form
		creds.Credentials.URL = urlEntry.Text
		creds.Credentials.Username = usernameEntry.Text
		creds.Credentials.Password = passwordEntry.Text

		f, err := os.Create(configPath)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		defer f.Close()

		if err := toml.NewEncoder(f).Encode(creds); err != nil {
			dialog.ShowError(err, w)
			return
		}
		dialog.ShowInformation("Success", "Configuration saved successfully!", w)
	})

	// --- Final Layout ---
	w.SetContent(container.NewBorder(
		nil, 
		container.NewVBox(widget.NewSeparator(), saveBtn),
		nil, nil,
		container.NewVSplit(credsForm, mappingsContainer),
	))
	w.Resize(fyne.NewSize(800, 600))
	w.ShowAndRun()
}
