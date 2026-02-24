package config

type Config struct {
	Databases []DatabaseConfig `json:"databases"`
	Interval  int              `json:"interval"` // 检查间隔（秒）
	Storage   StorageConfig    `json:"storage"`
}

type DatabaseConfig struct {
	Type     string `json:"type"` // 数据库类型，如 "postgres"
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DBName   string `json:"dbname"`
}

type StorageConfig struct {
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"` // 添加此行
	Password string `json:"password,omitempty"`
	DBName   string `json:"dbname,omitempty"`
}
