package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
)

//ConfigurationSettings contains the structure of all the settings that will be loaded at runtime.
type ConfigurationSettings struct {
	DBName                 string
	DBUser                 string
	DBPassword             string
	DBPort                 string
	DBHost                 string
	ImageDirectory         string
	Address                string
	ReadTimeout            time.Duration
	WriteTimeout           time.Duration
	MaxHeaderBytes         int
	SessionStoreKey        []byte
	HTTPRoot               string
	MaxUploadBytes         int64
	AllowAccountCreation   bool
	MaxThumbnailWidth      uint
	MaxThumbnailHeight     uint
	DefaultPermissions     uint64
	UsersControlOwnObjects bool
}

//SessionStore contains cookie information
var SessionStore *sessions.CookieStore

//Configuration contains all the information loaded from the config file.
var Configuration ConfigurationSettings

//ApplicationVersion Current version of application. This should be incremented every release
var ApplicationVersion = "0.0.0.3"

//SessionVariableName is used when checking cookies
var SessionVariableName = "gib-session"

//LoadConfiguration loads the specifed configuration file into Configuration
func LoadConfiguration(Path string) error {
	//Open the specifed file
	File, err := os.Open(Path)
	if err != nil {
		return err
	}
	defer File.Close()
	//Init a JSON Decoder
	decoder := json.NewDecoder(File)
	//Use decoder to decode into a ConfigrationSettings struct
	err = decoder.Decode(&Configuration)
	if err != nil {
		return err
	}
	return nil
}

//SaveConfiguration saves the configuration data in Configuration to the specified file path
func SaveConfiguration(Path string) error {
	//Open the specified file at Path
	File, err := os.OpenFile(Path, os.O_CREATE|os.O_RDWR, 0660)
	defer File.Close()
	if err != nil {
		return err
	}
	//Initialize an encoder to the File
	encoder := json.NewEncoder(File)
	//Encode the settings stored in configuration to File
	err = encoder.Encode(&Configuration)
	if err != nil {
		return err
	}
	return nil
}

//CreateSessionStore will create a new key store given a byte slice. If the slice is nil, a random key will be used.
func CreateSessionStore() {
	if Configuration.SessionStoreKey == nil {
		Configuration.SessionStoreKey = securecookie.GenerateRandomKey(64)
	}
	SessionStore = sessions.NewCookieStore(Configuration.SessionStoreKey)
}

//JoinPath is a utility function to join two system paths together regardless of whether the source ends in a slash
func JoinPath(A string, B string) string {
	if !strings.HasSuffix(A, string(filepath.Separator)) && !strings.HasPrefix(B, string(filepath.Separator)) {
		A = A + string(filepath.Separator)
	}
	if strings.HasSuffix(A, string(filepath.Separator)) && strings.HasPrefix(B, string(filepath.Separator)) {
		A = A[:len(A)-len(string(filepath.Separator))]
	}
	return A + B
}
