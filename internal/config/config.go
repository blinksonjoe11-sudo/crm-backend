package config

import (
	"log"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Port             string
	Env              string
	DatabaseURL      string
	RedisURL         string
	JWTSecret        string
	JWTRefreshSecret string
	JWTExpiry        time.Duration
	JWTRefreshExpiry time.Duration
	AllowedOrigins   []string
	CloudinaryCloudName string
	CloudinaryAPIKey    string
	CloudinaryAPISecret string

	MailjetAPIKeyPublic  string
	MailjetAPIKeyPrivate string
	MailFromEmail        string
	MailFromName         string


	GoogleClientID     string
	GoogleClientSecret string
	GoogleCallbackURL  string

	AppURL string
}

func Load() *Config {
	viper.SetConfigFile(".env")
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	_ = viper.ReadInConfig()

	viper.SetDefault("PORT", "8080")
	viper.SetDefault("ENV", "development")
	viper.SetDefault("JWT_EXPIRY", "15m")
	viper.SetDefault("JWT_REFRESH_EXPIRY", "720h")
	viper.SetDefault("SMTP_PORT", 587)
	viper.SetDefault("ALLOWED_ORIGINS", "http://localhost:3000")
	viper.SetDefault("APP_URL", "http://localhost:8080")

	cfg := &Config{
		Port:                 viper.GetString("PORT"),
		Env:                  viper.GetString("ENV"),
		DatabaseURL:          viper.GetString("DATABASE_URL"),
		RedisURL:             viper.GetString("REDIS_URL"),
		JWTSecret:            viper.GetString("JWT_SECRET"),
		JWTRefreshSecret:     viper.GetString("JWT_REFRESH_SECRET"),
		JWTExpiry:            viper.GetDuration("JWT_EXPIRY"),
		JWTRefreshExpiry:     viper.GetDuration("JWT_REFRESH_EXPIRY"),
		AllowedOrigins:       strings.Split(viper.GetString("ALLOWED_ORIGINS"), ","),
		CloudinaryCloudName:  viper.GetString("CLOUDINARY_CLOUD_NAME"),
		CloudinaryAPIKey:     viper.GetString("CLOUDINARY_API_KEY"),
		CloudinaryAPISecret:  viper.GetString("CLOUDINARY_API_SECRET"),
		MailjetAPIKeyPublic:  viper.GetString("MAILJET_API_KEY_PUBLIC"),
		MailjetAPIKeyPrivate: viper.GetString("MAILJET_API_KEY_PRIVATE"),
		MailFromEmail:        viper.GetString("MAIL_FROM_EMAIL"),
		MailFromName:         viper.GetString("MAIL_FROM_NAME"),
		GoogleClientID:       viper.GetString("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:   viper.GetString("GOOGLE_CLIENT_SECRET"),
		GoogleCallbackURL:    viper.GetString("GOOGLE_CALLBACK_URL"),
		AppURL:               viper.GetString("APP_URL"),
	}

	for k, v := range map[string]string{
		"DATABASE_URL": cfg.DatabaseURL, "REDIS_URL": cfg.RedisURL,
		"JWT_SECRET": cfg.JWTSecret, "JWT_REFRESH_SECRET": cfg.JWTRefreshSecret,
	} {
		if v == "" {
			log.Fatalf("config: required env var %s is missing", k)
		}
	}
	return cfg
}
