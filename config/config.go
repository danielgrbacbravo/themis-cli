package config

import (
	"log"
	"net/url"
	"os"

	"github.com/joho/godotenv"
)

func GenerateLoginURLValuesFromENV(csrfToken string) (loginData url.Values, err error) {
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

func GetUsernameFromENV() (username string) {
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

func SetUsernameInENV(username string) {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	os.Setenv("USERNAME", username)
}

func SetPasswordInENV(password string) {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	os.Setenv("PASSWORD", password)
}

func GenerateENVFile() {
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

func DoesENVFileExist() bool {
	if _, err := os.Stat(".env"); os.IsNotExist(err) {
		return false
	}
	return true
}

func IsENVFileEmpty() bool {
	if GetUsernameFromENV() == "" || GetPasswordFromENV() == "" {
		return true
	}
	return false
}
