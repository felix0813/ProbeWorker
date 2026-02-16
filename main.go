package main

import (
	"ProbeWorker/checker"
	"ProbeWorker/config"
	"ProbeWorker/storage"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

var (
	currentConfig *config.Config
	mu            sync.RWMutex
)

func loadConfig(filename string) (*config.Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func runChecks(cfg *config.Config, storage storage.Storage) {
	ctx := context.Background()
	for _, dbCfg := range cfg.Databases {
		var databaseChecker checker.DatabaseChecker
		switch dbCfg.Type {
		case "postgres":
			databaseChecker = checker.NewPostgresCheckerWithStorage(
				dbCfg.Host, dbCfg.Port, dbCfg.User, dbCfg.Password, dbCfg.DBName, storage)
		default:
			log.Printf("不支持的数据库类型: %s", dbCfg.Type)
			continue
		}

		go func(c checker.DatabaseChecker) {
			ticker := time.NewTicker(time.Duration(cfg.Interval) * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := c.Check(ctx); err != nil {
						log.Printf("[%s] 检查失败: %v", c.Name(), err)
					}
				case <-ctx.Done():
					return
				}
			}
		}(databaseChecker)
	}
}

func watchConfig(filename string, reload chan<- struct{}) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	if err := watcher.Add(filename); err != nil {
		log.Fatal(err)
	}

	for {
		select {
		case event := <-watcher.Events:
			if event.Op&fsnotify.Write == fsnotify.Write {
				reload <- struct{}{}
			}
		case err := <-watcher.Errors:
			log.Println("配置监听错误:", err)
		}
	}
}

func main() {
	cfgFile := "config.json"
	cfg, err := loadConfig(cfgFile)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化存储
	storageCfg := cfg.Storage
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		storageCfg.Host, storageCfg.Port, storageCfg.User, storageCfg.Password, storageCfg.DBName)
	storage, err := storage.NewPostgresStorage(connStr)
	if err != nil {
		log.Fatalf("初始化存储失败: %v", err)
	}

	mu.Lock()
	currentConfig = cfg
	mu.Unlock()

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	reloadChan := make(chan struct{})
	go watchConfig(cfgFile, reloadChan)

	runChecks(currentConfig, storage) // 传入 storage 实例

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-sigChan:
			fmt.Println("程序退出")
			return
		case <-reloadChan:
			fmt.Println("检测到配置变更，重新加载...")
			newCfg, err := loadConfig(cfgFile)
			if err != nil {
				log.Printf("重载配置失败: %v", err)
				continue
			}
			mu.Lock()
			currentConfig = newCfg
			mu.Unlock()
			cancel()
			_, cancel = context.WithCancel(context.Background())
			runChecks(currentConfig, storage) // 重新传入 storage 实例
		}
	}
}
