package bootstrap

import (
	"log"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"meta-api/config"
)

// initConfig 初始化配置
func initConfig() *config.Config {
	var err error
	viper.SetConfigType("yaml")
	viper.SetConfigFile("./config/config.yml")

	// 读取配置信息
	if err = viper.ReadInConfig(); err != nil {
		log.Panicf("Read config.yml file error: %v", err)
		return nil
	}

	// 将读取到的配置信息反序列化到 Config 中
	var cfg config.Config
	if err = viper.Unmarshal(&cfg); err != nil {
		log.Panicf("Viper unmarshal error: %v", err)
		return &cfg
	}

	// 监视配置文件变化
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		if err = viper.Unmarshal(&cfg); err != nil {
			log.Panicf("Viper unmarshal error: %v", err)
			return
		}
	})

	return &cfg
}
