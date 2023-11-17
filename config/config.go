package config

import (
	"log"
	"net/url"
	"os"

	"github.com/joho/godotenv"
)

func GenerateLoginURLValuesFromENV(csrfToken string) (loginData url.Values, err error) {
	CheckENVFile()
	loginData = url.Values{
		"user":     {GetUsernameFromENV()},
		"password": {GetPasswordFromENV()},
		"_csrf":    {csrfToken},
	}
	if loginData.Get("user") == "" || loginData.Get("password") == "" {
		return loginData, err

	}
	return loginData, nil
}

func GetUsernameFromENV() string {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	return os.Getenv("USERNAME")
}

func GetPasswordFromENV() string {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	return os.Getenv("PASSWORD")
}

func generateENVFile() {
	f, err := os.Create(".env")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	_, err = f.WriteString("USERNAME=\nPASSWORD=")
	if err != nil {
		log.Fatal(err)
	}
}

func CheckENVFile() {
	if _, err := os.Stat(".env"); os.IsNotExist(err) {
		generateENVFile()
	}
}
