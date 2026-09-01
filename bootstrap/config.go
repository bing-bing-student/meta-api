package bootstrap

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	commentModeration "meta-api/app/service/comment/moderation"
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
	"./config/comment_moderation.manifest.yml",
}

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
	return loadConfigFileSet(configFiles)
}

// loadConfigFileSet 先加载应用入口配置，再根据 comment_moderation.policy_files
// 递归合并审核策略包。映射按键合并、数组按声明顺序追加、标量由后加载文件覆盖。
// 这样领域规则可以拆分到独立文件，同时保持最终反序列化结果仍是一份强类型配置。
func loadConfigFileSet(baseFiles []string) (*config.Config, []string, error) {
	settings := make(map[string]any)
	files := append([]string(nil), baseFiles...)
	for _, file := range baseFiles {
		fragment, err := readConfigSettings(file)
		if err != nil {
			return nil, nil, err
		}
		mergeConfigSettings(settings, fragment)
	}

	policyFiles, err := resolveModerationPolicyFiles(baseFiles, moderationPolicyFileNames(settings))
	if err != nil {
		return nil, nil, err
	}
	for _, file := range policyFiles {
		fragment, readErr := readConfigSettings(file)
		if readErr != nil {
			return nil, nil, readErr
		}
		mergeConfigSettings(settings, fragment)
		files = append(files, file)
	}

	reader := viper.New()
	if err = reader.MergeConfigMap(settings); err != nil {
		return nil, nil, fmt.Errorf("merge config settings: %w", err)
	}

	var next config.Config
	if err := reader.Unmarshal(&next); err != nil {
		return nil, nil, err
	}
	if next.CommentModerationConfig != nil {
		policy := *next.CommentModerationConfig
		if err := commentModeration.ValidateConfig(policy); err != nil {
			return nil, nil, fmt.Errorf("validate comment moderation policy: %w", err)
		}
	}
	return &next, files, nil
}

func readConfigSettings(file string) (map[string]any, error) {
	reader := viper.New()
	reader.SetConfigType("yaml")
	reader.SetConfigFile(file)
	if err := reader.ReadInConfig(); err != nil {
		return nil, err
	}
	return reader.AllSettings(), nil
}

func mergeConfigSettings(dst, src map[string]any) {
	for key, value := range src {
		if sourceMap, ok := value.(map[string]any); ok {
			if destinationMap, exists := dst[key].(map[string]any); exists {
				mergeConfigSettings(destinationMap, sourceMap)
				continue
			}
			copied := make(map[string]any, len(sourceMap))
			mergeConfigSettings(copied, sourceMap)
			dst[key] = copied
			continue
		}
		if sourceSlice, ok := value.([]any); ok {
			if destinationSlice, exists := dst[key].([]any); exists {
				dst[key] = append(destinationSlice, sourceSlice...)
			} else {
				dst[key] = append([]any(nil), sourceSlice...)
			}
			continue
		}
		dst[key] = value
	}
}

func moderationPolicyFileNames(settings map[string]any) []string {
	moderation, ok := settings["comment_moderation"].(map[string]any)
	if !ok {
		return nil
	}
	values, ok := moderation["policy_files"].([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if file, ok := value.(string); ok && strings.TrimSpace(file) != "" {
			result = append(result, strings.TrimSpace(file))
		}
	}
	return result
}

func resolveModerationPolicyFiles(baseFiles, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	configDir := "./config"
	for _, file := range baseFiles {
		if strings.HasPrefix(filepath.Base(file), "comment_moderation") {
			configDir = filepath.Dir(file)
			break
		}
	}
	root, err := filepath.Abs(configDir)
	if err != nil {
		return nil, fmt.Errorf("resolve moderation policy root: %w", err)
	}
	result := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		candidate, absErr := filepath.Abs(filepath.Join(root, filepath.Clean(name)))
		if absErr != nil {
			return nil, fmt.Errorf("resolve moderation policy file %q: %w", name, absErr)
		}
		relative, relErr := filepath.Rel(root, candidate)
		if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("comment_moderation.policy_files path %q escapes config directory", name)
		}
		if _, exists := seen[candidate]; exists {
			return nil, fmt.Errorf("comment_moderation.policy_files contains duplicate %q", name)
		}
		seen[candidate] = struct{}{}
		result = append(result, candidate)
	}
	return result, nil
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
