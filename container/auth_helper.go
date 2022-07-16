package container

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"rcon/utils"
)

var (
	errAuthNotFound = errors.New("no auth creds found")
	errNoAuthFile   = errors.New("no auth file")
)

type creds struct {
	Username string `json:"username"`
	Secret   string `json:"secret"`
}

type AuthHelper struct {
	AuthFile string
}

func (a *AuthHelper) Get(serverURL string) (string, string, error) {
	if a.AuthFile == "" {
		return "", "", errNoAuthFile
	}

	data, err := ioutil.ReadFile(a.AuthFile)
	if err != nil {
		return "", "", err
	}

	mapCfg := make(map[string]creds)
	err = json.Unmarshal(data, &mapCfg)
	if err != nil {
		return "", "", err
	}

	// check if imageRef exists
	if c, ok := mapCfg[serverURL]; ok {
		return c.Username, c.Secret, nil
	}

	return "", "", errAuthNotFound
}

func (a *AuthHelper) Add(serverUrl, username, secret string) error {
	if a.AuthFile == "" {
		return errNoAuthFile
	}

	mapCfg := make(map[string]creds)

	if utils.PathExists(a.AuthFile) {
		data, err := ioutil.ReadFile(a.AuthFile)
		if err != nil {
			return err
		}

		err = json.Unmarshal(data, &mapCfg)
		if err != nil {
			return err
		}
	}

	mapCfg[serverUrl] = creds{Username: username, Secret: secret}
	data, err := json.Marshal(mapCfg)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(a.AuthFile, data, 0600)
}
