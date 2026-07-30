package bootstrap

import (
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"meta-api/config"
)

var configFiles = []string{
	"./config/app.yml",
	"./config/rate_limit.yml",
	"./config/comment_moderation.yml",
}

const legacyConfigFile = "./config/config.yml"

// initConfig 初始化配置
func initConfig() *config.Config {
	cfg := &config.Config{}
	initial, files, err := loadConfigFiles()
	if err != nil {
		log.Panicf("Read config files error: %v", err)
		return nil
	}
	cfg.Replace(initial)
	watchConfigFiles(cfg, files)

	return cfg
}

func loadConfigFiles() (*config.Config, []string, error) {
	files := configFiles
	if !allConfigFilesExist(files) {
		files = []string{legacyConfigFile}
	}

	reader := viper.New()
	reader.SetConfigType("yaml")
	for index, file := range files {
		reader.SetConfigFile(file)
		var err error
		if index == 0 {
			err = reader.ReadInConfig()
		} else {
			err = reader.MergeInConfig()
		}
		if err != nil {
			return nil, nil, err
		}
	}

	var next config.Config
	if err := reader.Unmarshal(&next); err != nil {
		return nil, nil, err
	}
	return &next, files, nil
}

func allConfigFilesExist(files []string) bool {
	for _, file := range files {
		if _, err := os.Stat(file); err != nil {
			return false
		}
	}
	return true
}

func watchConfigFiles(cfg *config.Config, files []string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Config watch disabled: %v", err)
		return
	}
	for _, file := range files {
		if err := watcher.Add(filepath.Clean(file)); err != nil {
			log.Printf("Watch config file %s error: %v", file, err)
		}
	}

	go func() {
		defer func() {
			if err := watcher.Close(); err != nil && !errors.Is(err, fsnotify.ErrEventOverflow) {
				log.Printf("Close config watcher error: %v", err)
			}
		}()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
					continue
				}
				next, _, err := loadConfigFiles()
				if err != nil {
					log.Printf("Reload config files error, keep old config: %v", err)
					continue
				}
				cfg.Replace(next)
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("Watch config files error: %v", err)
			}
		}
	}()
}
