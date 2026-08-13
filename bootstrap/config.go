package bootstrap

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"meta-api/config"
)

// 配置热更新边界：
//   - 支持热更新：oauth、admin_info、bug_feedback、rate_limit、comment_moderation。
//     这些配置在运行期通过 Config.*Snapshot 方法读取，替换后可被后续请求感知。
//   - 仅启动期生效：log、retry、mysql、redis、article_image、guard，以及 HTTP/env。
//     这些配置用于构造 logger、连接池、外部客户端或 guard.Engine；修改后需要重启进程。
//
// watchConfigFiles 只会把支持热更新的配置段写回 cfg，避免出现"配置对象变了，
// 但启动期组件没有重建"的误导性状态。
var configFiles = []string{
	"./config/app.yml",
	"./config/rate_limit.yml",
	"./config/comment_moderation.yml",
}

const legacyConfigFile = "./config/config.yml"

// initConfig 初始化配置
func initConfig() (*config.Config, *ConfigWatcher, error) {
	cfg := &config.Config{}
	initial, files, err := loadConfigFiles()
	if err != nil {
		return nil, nil, err
	}
	cfg.Replace(initial)
	watcher, err := watchConfigFiles(cfg, files)
	if err != nil {
		log.Printf("Config watch disabled: %v", err)
		return cfg, nil, nil
	}

	return cfg, watcher, nil
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

// ConfigWatcher 管理配置文件 fsnotify watcher 的生命周期。
type ConfigWatcher struct {
	watcher  *fsnotify.Watcher
	done     chan struct{}
	closeErr error
	once     sync.Once
}

func watchConfigFiles(cfg *config.Config, files []string) (*ConfigWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create config watcher: %w", err)
	}

	watched := 0
	for _, file := range files {
		if err := watcher.Add(filepath.Clean(file)); err != nil {
			log.Printf("Watch config file %s error: %v", file, err)
			continue
		}
		watched++
	}
	if watched == 0 {
		if closeErr := watcher.Close(); closeErr != nil {
			return nil, errors.Join(fmt.Errorf("no config files watched"), closeErr)
		}
		return nil, fmt.Errorf("no config files watched")
	}

	cw := &ConfigWatcher{
		watcher: watcher,
		done:    make(chan struct{}),
	}
	go func() {
		defer close(cw.done)
		for {
			select {
			case event, ok := <-cw.watcher.Events:
				if !ok {
					return
				}
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
					continue
				}
				next, _, err := loadConfigFiles()
				if err != nil {
					log.Printf("Reload config files error, keep old hot-reloadable config: %v", err)
					continue
				}
				cfg.ReplaceHotReloadable(next)
			case err, ok := <-cw.watcher.Errors:
				if !ok {
					return
				}
				log.Printf("Watch config files error: %v", err)
			}
		}
	}()
	return cw, nil
}

// Close 停止配置文件监听。
func (w *ConfigWatcher) Close() error {
	if w == nil || w.watcher == nil {
		return nil
	}
	w.once.Do(func() {
		w.closeErr = w.watcher.Close()
		select {
		case <-w.done:
		case <-time.After(time.Second):
			w.closeErr = errors.Join(w.closeErr, fmt.Errorf("config watcher shutdown timed out"))
		}
	})
	if errors.Is(w.closeErr, fsnotify.ErrEventOverflow) {
		return nil
	}
	return w.closeErr
}
