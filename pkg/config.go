package internal

import (
	"encoding/json"
	"os"
)

// ConfigStruct Структура конфигурации приложения
type ConfigStruct struct {
	DBConfigs []DBConfig `json:"databases"`
}

// Config Глобальная конфигурация приложения
var Config ConfigStruct

//DBConfig Структура данных о БД
type DBConfig struct {
	Name       string   `json:"name"`
	BackupCMD  []string `json:"backup_cmd"`
	RestoreCMD []string `json:"restore_cmd"`
	BackupDir  string   `json:"backup_dir"`
	BackupFreq string   `json:"backup_freq"`
}

//Load Загрузка конфигурации из заданного файла
func Load(configPath string) (*ConfigStruct, error) {
	configFile, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer configFile.Close()
	decoder := json.NewDecoder(configFile)

	if err := decoder.Decode(&Config); err != nil {
		return nil, err
	}
	return &Config, nil
}
