package main

// cleg_image：image/png 解码 + 路径缓存（同一路径只解码一次）+ 4 种 repeat。
//
// 精灵保存为 0x00RRGGBB + 独立 alpha（straight alpha，逐像素 source-over），
// 与帧缓冲 v1 的 RGB 直写约定一致；缩放用最近邻（性能优先，GUI 背景图足够）。

import (
	"image/color"
	"image/png"
	"os"
	"sync"
)

// sprite 解码后的 PNG 精灵。
type sprite struct {
	w, h int
	pix  []uint32 // 0x00RRGGBB
	alp  []uint8  // 0..255
}

var (
	spriteMu    sync.Mutex
	spriteCache = map[string]*sprite{}
)

// loadSprite 按路径解码 PNG 并缓存（并发安全；同一路径只解码一次）。
func loadSprite(path string) (*sprite, error) {
	if path == "" {
		return nil, os.ErrNotExist
	}
	spriteMu.Lock()
	if s, ok := spriteCache[path]; ok {
		spriteMu.Unlock()
		return s, nil
	}
	spriteMu.Unlock()
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	s := &sprite{w: b.Dx(), h: b.Dy()}
	if s.w <= 0 || s.h <= 0 {
		return nil, os.ErrInvalid
	}
	s.pix = make([]uint32, s.w*s.h)
	s.alp = make([]uint8, s.w*s.h)
	for y := 0; y < s.h; y++ {
		for x := 0; x < s.w; x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			i := y*s.w + x
			s.pix[i] = uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
			s.alp[i] = c.A
		}
	}
	spriteMu.Lock()
	if old, ok := spriteCache[path]; ok { // 竞态下保留先到者
		spriteMu.Unlock()
		return old, nil
	}
	spriteCache[path] = s
	spriteMu.Unlock()
	return s, nil
}

// blitSpriteScaled 最近邻缩放绘制（裁剪感知）。
func (fb *framebuffer) blitSpriteScaled(s *sprite, x, y, w, h int) {
	if s.w <= 0 || s.h <= 0 || w <= 0 || h <= 0 {
		return
	}
	for yy := 0; yy < h; yy++ {
		dy := y + yy
		if dy < fb.clipY0 || dy >= fb.clipY1 {
			continue
		}
		sy := yy * s.h / h
		base := sy * s.w
		for xx := 0; xx < w; xx++ {
			dx := x + xx
			if dx < fb.clipX0 || dx >= fb.clipX1 {
				continue
			}
			i := base + xx*s.w/w
			a := int(s.alp[i])
			if a == 0 {
				continue
			}
			fb.putPixel(dx, dy, s.pix[i], a)
		}
	}
}

// blitSpriteTiles 以自然尺寸平铺，区域为 [x,x1) × [y,y1)（裁剪感知）。
func (fb *framebuffer) blitSpriteTiles(s *sprite, x, y, x1, y1 int) {
	if s.w <= 0 || s.h <= 0 {
		return
	}
	for ty := y; ty < y1; ty += s.h {
		for tx := x; tx < x1; tx += s.w {
			for yy := 0; yy < s.h; yy++ {
				dy := ty + yy
				if dy < fb.clipY0 || dy >= fb.clipY1 || dy >= y1 {
					continue
				}
				for xx := 0; xx < s.w; xx++ {
					dx := tx + xx
					if dx < fb.clipX0 || dx >= fb.clipX1 || dx >= x1 {
						continue
					}
					i := yy*s.w + xx
					a := int(s.alp[i])
					if a == 0 {
						continue
					}
					fb.putPixel(dx, dy, s.pix[i], a)
				}
			}
		}
	}
}

// imageCore cleg_image 内核：repeat 0=no-repeat 1=repeat 2=repeat-x 3=repeat-y。
// no-repeat：w/h>0 时缩放到 w×h，否则用图片自然尺寸；
// repeat*：瓦片用自然尺寸，平铺区域为 w/h>0 的 [x,x+w)×[y,y+h)，否则到当前裁剪区边界。
func (fb *framebuffer) imageCore(path string, x, y, w, h, repeat int) int {
	s, err := loadSprite(path)
	if err != nil {
		return -2
	}
	x, y, w, h = fb.xfRect(x, y, w, h)
	if repeat <= 0 {
		if w <= 0 || h <= 0 {
			w, h = s.w, s.h
		}
		fb.blitSpriteScaled(s, x, y, w, h)
		return 0
	}
	x1, y1 := fb.clipX1, fb.clipY1
	if w > 0 {
		x1 = x + w
	}
	if h > 0 {
		y1 = y + h
	}
	switch repeat {
	case 2: // repeat-x：单行
		fb.blitSpriteTiles(s, x, y, x1, y+s.h)
	case 3: // repeat-y：单列
		fb.blitSpriteTiles(s, x, y, x+s.w, y1)
	default: // repeat：双向
		fb.blitSpriteTiles(s, x, y, x1, y1)
	}
	return 0
}
