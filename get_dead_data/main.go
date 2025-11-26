//go:build !config_gui && !captcha_gui

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	log.Println("Starting Deceased Patient Search Tool...")

	// --- Config Loading ---
	profiles, err := LoadProfilesConfig("profiles.toml")
	if err != nil {
		log.Fatalf("FATAL: Could not load profiles.toml: %v", err)
	}
	creds, err := LoadCredentialsConfig("credentials.toml")
	if err != nil {
		log.Fatalf("FATAL: Could not load credentials.toml: %v", err)
	}
	log.Println("Configuration loaded.")

	// --- URL Mapping and Client Setup ---
	client := NewClient(creds.Credentials, creds.URLMappings)

	log.Println("Detecting login page type...")
	profile, profileName, err := client.DetectLoginProfile(profiles)
	if err != nil {
		log.Fatalf("FATAL: Could not detect login profile: %v", err)
	}
	log.Printf("Detected Profile: '%s'.", profileName)

	// --- Captcha Handling (if needed) ---
	var captchaCode string
	if profile.CaptchaEnabled {
		log.Println("Captcha required, launching GUI...")
		captchaBytes, err := client.GetCaptcha()
		if err != nil {
			log.Fatalf("FATAL: Could not fetch captcha: %v", err)
		}

		// Save captcha to a temporary file
		tmpFile, err := ioutil.TempFile(os.TempDir(), "captcha-*.png")
		if err != nil {
			log.Fatalf("FATAL: Could not create temp file for captcha: %v", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.Write(captchaBytes); err != nil {
			log.Fatalf("FATAL: Could not write captcha to temp file: %v", err)
		}
		tmpFile.Close()

		// Get the path to the current executable to find the captcha_gui
		exePath, err := os.Executable()
		if err != nil {
			log.Fatalf("FATAL: Could not find executable path: %v", err)
		}
		captchaAppPath := filepath.Join(filepath.Dir(exePath), "captcha_gui")

		// Launch the captcha GUI as a subprocess
		cmd := exec.Command(captchaAppPath, tmpFile.Name())
		output, err := cmd.Output()
		if err != nil {
			log.Fatalf("FATAL: Captcha GUI failed. Make sure you have built it with 'go build -tags=captcha_gui -o captcha_gui'. Error: %v", err)
		}
		captchaCode = string(output)
		log.Println("Captcha received from GUI.")
	}

	// --- Login ---
	log.Println("Logging in...")
	err = client.Login(creds.Credentials.Username, creds.Credentials.Password, captchaCode, profileName)
	if err != nil {
		log.Fatalf("FATAL: Login failed: %v", err)
	}

	// --- Search ---
	log.Println("Login successful. Searching for deceased patients...")
	patients, err := client.SearchDeadPatients()
	if err != nil {
		log.Fatalf("FATAL: Search failed: %v", err)
	}

	// --- Display Results ---
	if len(patients) == 0 {
		log.Println("Search complete. No deceased patients found.")
		return
	}

	fmt.Println("\n--- Deceased Patient Search Results ---")
	for _, p := range patients {
		fmt.Printf("Name: %s, ID Number: %s\n", p.PersonName, p.IdNumber)
	}
	fmt.Println("--------------------------------------")
}
