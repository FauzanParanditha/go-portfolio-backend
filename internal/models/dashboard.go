package models

import "time"

type DashboardOverviewResponse struct {
	Projects struct {
		Total       int64 `json:"total"`
		Featured    int64 `json:"featured"`
		RecentCount int64 `json:"recentCount"`
	} `json:"projects"`

	Experiences struct {
		Total       int64 `json:"total"`
		Current     int64 `json:"current"`
		RecentCount int64 `json:"recentCount"`
	} `json:"experiences"`

	ContactMessages struct {
		Total       int64 `json:"total"`
		Unread      int64 `json:"unread"`
		RecentCount int64 `json:"recentCount"`
	} `json:"contactMessages"`

	System struct {
		ServerTime time.Time `json:"serverTime"`
		RecentDays int       `json:"recentDays"`
	} `json:"system"`
}