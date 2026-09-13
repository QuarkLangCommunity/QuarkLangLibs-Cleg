package main

import (
	"os"
	"strconv"
)

func itoa(n int) string { return strconv.Itoa(n) }

// loadFontFile 按路径加载 TTF（带缓存；CJK/任意字体文件）。
func loadFontFile(path string, px int) (*ttfFont, error) {
	ttfCacheMu.Lock()
	defer ttfCacheMu.Unlock()
	key := path + "|" + itoa(px)
	if f, ok := ttfCache[key]; ok {
		return f, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := parseTTF(data)
	if err != nil {
		return nil, err
	}
	f.path = path
	ttfCache[key] = f
	return f, nil
}

func osReadFile(p string) ([]byte, error) { return os.ReadFile(p) }

func fontOf(name string) string { return fontPathOf(name) }
