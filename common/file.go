package common

import "time"

type File struct {
	Path    string    `json:"path"`
	ModTime time.Time `json:"mod_time"`
	Size    int64     `json:"size"`
	IsDir   bool      `json:"is_dir"`
}
