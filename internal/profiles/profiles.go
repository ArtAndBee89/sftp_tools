package profiles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Profile struct {
	Name           string `json:"name"`
	Host           string `json:"host"`
	User           string `json:"user"`
	Password       string `json:"password,omitempty"`
	KeyPath        string `json:"keyPath,omitempty"`
	ProxyHost      string `json:"proxyHost,omitempty"`
	ProxyUser      string `json:"proxyUser,omitempty"`
	ProxyPassword  string `json:"proxyPassword,omitempty"`
	ProxyKeyPath   string `json:"proxyKeyPath,omitempty"`
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sftp-gui", "profiles.json"), nil
}

func Load() ([]Profile, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var list []Profile
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func Save(list []Profile) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// Add добавляет профиль или заменяет существующий с тем же именем.
func Add(p Profile) ([]Profile, error) {
	list, err := Load()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Name == p.Name {
			list[i] = p
			return list, Save(list)
		}
	}
	list = append(list, p)
	return list, Save(list)
}

// Remove удаляет профиль по имени. Возвращает ошибку, если профиль не найден.
func Remove(name string) ([]Profile, error) {
	list, err := Load()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Name == name {
			list = append(list[:i], list[i+1:]...)
			return list, Save(list)
		}
	}
	return nil, fmt.Errorf("профиль %q не найден", name)
}