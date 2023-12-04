package config

import (
	"net/url"
	"os"

	log "github.com/charmbracelet/log"
	"github.com/joho/godotenv"
)

func init() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("No .env file found")
	}
}

func GetIDFromENV() string {
	return os.Getenv("ID")
}

func GetPasswordFromENV() string {
	return os.Getenv("PASSWORD")
}

func GenerateLoginURLValuesFromENV(csrfToken string) (loginData url.Values, err error) {
	loginData = url.Values{
		"user":     {GetIDFromENV()},
		"password": {GetPasswordFromENV()},
		"_csrf":    {csrfToken},
	}
	log.Debug("Login Data", "loginData", loginData)
	if loginData.Get("user") == "" || loginData.Get("password") == "" {
		return loginData, err

	}
	return loginData, nil
}
