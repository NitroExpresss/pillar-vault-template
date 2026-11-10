// Витрина ориентиров: слайды из viewer/gallery/config.json и замена фото
// прямо с дашборда — с телефона, без git и без доступа к серверу.
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type gallerySlide struct {
	Src     string `json:"src"`
	Caption string `json:"caption"`
	Sub     string `json:"sub"`
}

type galleryConfig struct {
	Title      string         `json:"title,omitempty"`
	Note       string         `json:"note,omitempty"`
	AutoplayMs *int           `json:"autoplayMs,omitempty"` // нет — 7 с в app.js, 0 — без автопрокрутки
	Slides     []gallerySlide `json:"slides"`
}

const (
	galleryDir        = "viewer/gallery"
	galleryConfigFile = galleryDir + "/config.json"
	maxPhotoBytes     = 4 << 20 // столько пропускает nginx (client_max_body_size 4m); браузер ужимает фото заранее
)

// Файлы, которыми сервер распоряжается сам: загруженные с веба фото и
// примеры-моки из репозитория. Их он удаляет, когда слайд убран или фото
// заменено. Файлы, положенные в папку руками под другими именами, не трогает.
const (
	uploadedPrefix = "photo-"
	mockPrefix     = "mock-"
)

// isMock — слайд-пример, заглушка до настоящих фото.
func (s gallerySlide) IsMock() bool { return strings.HasPrefix(path.Base(s.Src), mockPrefix) }

// Mocks — сколько примеров ещё висит: дашборд предлагает убрать их разом.
func (c *galleryConfig) Mocks() int {
	n := 0
	for _, s := range c.Slides {
		if s.IsMock() {
			n++
		}
	}
	return n
}

func readGallery() (*galleryConfig, error) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(galleryConfigFile)))
	if err != nil {
		return nil, err
	}
	var cfg galleryConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// loadGallery — для шаблона. Файла нет — панели нет. Слайдов нет — панель
// есть, но только с формой: первое фото тоже загружается отсюда.
func loadGallery() *galleryConfig {
	cfg, err := readGallery()
	if err != nil {
		return nil
	}
	slides := cfg.Slides[:0]
	for _, s := range cfg.Slides {
		if s.Src != "" {
			slides = append(slides, s)
		}
	}
	cfg.Slides = slides
	return cfg
}

// handleGallery: do=save — заменить фото и подписи слайда (slot=N) или
// добавить новый (slot=new); do=remove — убрать слайд; do=mocks — убрать
// все слайды-примеры разом. Принимаются только картинки, тип определяется
// по содержимому, а не по имени файла.
//
// Слайд адресуется номером, а номер едет, если витрину успели поправить в
// другой вкладке. Поэтому форма несёт и src слайда: не совпал — ничего не
// трогаем, иначе ✕ убрал бы соседний слайд.
func handleGallery(w http.ResponseWriter, r *http.Request) {
	if !requirePage(w, r) { // приложению и виджетам — 401, а не редирект, похожий на успех
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "только POST", http.StatusMethodNotAllowed)
		return
	}
	fail := func(msg string) { http.Redirect(w, r, urlWithMsg("/", msg), http.StatusSeeOther) }
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBytes)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		fail("фото не принято: больше 4 МБ или битая форма")
		return
	}
	defer r.MultipartForm.RemoveAll() // большие части формы Go держит во временных файлах
	cfg, err := readGallery()
	if err != nil {
		fail("нет viewer/gallery/config.json")
		return
	}

	touched := []string{galleryConfigFile}
	done := func(what, msg string) {
		if err := writeGallery(cfg); err != nil {
			fail("витрина не сохранена")
			return
		}
		commitFiles(what, touched...)
		markOK(w)
		http.Redirect(w, r, urlWithMsg("/", msg), http.StatusSeeOther)
	}

	if r.FormValue("do") == "mocks" {
		var kept, gone []gallerySlide
		for _, s := range cfg.Slides {
			if s.IsMock() {
				gone = append(gone, s)
			} else {
				kept = append(kept, s)
			}
		}
		if len(gone) == 0 {
			fail("примеров в витрине уже нет")
			return
		}
		cfg.Slides = kept
		for _, s := range gone {
			touched = append(touched, dropFile(s.Src, cfg)...)
		}
		done("витрина: примеры убраны", fmt.Sprintf("примеры убраны: %d", len(gone)))
		return
	}

	slot := -1 // новый слайд
	if s := r.FormValue("slot"); s != "new" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 || n >= len(cfg.Slides) {
			fail("нет такого слайда")
			return
		}
		if src := r.FormValue("src"); src != "" && src != cfg.Slides[n].Src {
			fail("витрину успели поправить — обновите страницу")
			return
		}
		slot = n
	}

	if r.FormValue("do") == "remove" {
		if slot < 0 {
			fail("удалять нечего")
			return
		}
		gone := cfg.Slides[slot]
		cfg.Slides = append(cfg.Slides[:slot], cfg.Slides[slot+1:]...)
		touched = append(touched, dropFile(gone.Src, cfg)...)
		done("витрина: убран слайд «"+gone.Caption+"»", "слайд убран")
		return
	}

	slide := gallerySlide{}
	if slot >= 0 {
		slide = cfg.Slides[slot]
	}
	slide.Caption = oneLine(r.FormValue("caption"), 80)
	slide.Sub = oneLine(r.FormValue("sub"), 120)

	replaced := "" // прежнее фото слайда, если его заменили
	file, _, err := r.FormFile("photo")
	switch {
	case err == nil:
		defer file.Close()
		src, err := savePhoto(file)
		if err != nil {
			fail(err.Error())
			return
		}
		replaced = slide.Src
		slide.Src = strings.TrimPrefix(src, "viewer/")
		touched = append(touched, src)
	case slot < 0:
		fail("для нового слайда нужно фото")
		return
	}

	if slot < 0 {
		cfg.Slides = append(cfg.Slides, slide)
	} else {
		cfg.Slides[slot] = slide
	}
	if err := writeGallery(cfg); err != nil {
		fail("витрина не сохранена")
		return
	}
	// прежний файл — только после записи конфига: сорвись запись, слайд
	// остался бы ссылкой в пустоту
	touched = append(touched, dropFile(replaced, cfg)...)
	commitFiles("витрина: «"+slide.Caption+"»", touched...)
	markOK(w)
	http.Redirect(w, r, urlWithMsg("/", "витрина обновлена"), http.StatusSeeOther)
}

// savePhoto кладёт картинку в viewer/gallery под уникальным именем: старое
// имя браузер мог закэшировать, новое — нет. Возвращает путь от корня.
func savePhoto(f io.Reader) (string, error) {
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif"}[http.DetectContentType(head[:n])]
	if ext == "" {
		return "", fmt.Errorf("это не фото: нужен JPEG, PNG, WebP или GIF")
	}
	// время плюс случайный хвост: часы на Windows грубые, и две загрузки
	// подряд получали одно имя — второе фото молча затирало первое
	tail := make([]byte, 4)
	rand.Read(tail)
	rel := path.Join(galleryDir, fmt.Sprintf("%s%d-%x%s", uploadedPrefix, time.Now().Unix(), tail, ext))
	full := filepath.Join(root, filepath.FromSlash(rel))
	out, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("фото не сохранено")
	}
	_, err = io.Copy(out, io.MultiReader(bytes.NewReader(head[:n]), f))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(full)
		return "", fmt.Errorf("фото не сохранено")
	}
	return rel, nil
}

// dropFile удаляет файл убранного или заменённого слайда — если им
// распоряжается сервер (photo-*, mock-*) и на него больше не ссылается ни
// один слайд витрины cfg. Файлы, положенные руками, не трогаются; путь за
// пределы папки витрины не выходит. Возвращает путь для коммита: удаление
// мока из репозитория тоже уходит в git, откатить его можно оттуда.
func dropFile(src string, cfg *galleryConfig) []string {
	if src == "" {
		return nil
	}
	rel := path.Join("viewer", path.Clean("/"+src))
	base := path.Base(rel)
	if path.Dir(rel) != galleryDir || !(strings.HasPrefix(base, uploadedPrefix) || strings.HasPrefix(base, mockPrefix)) {
		return nil
	}
	if cfg != nil {
		for _, s := range cfg.Slides {
			if path.Join("viewer", path.Clean("/"+s.Src)) == rel {
				return nil
			}
		}
	}
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		return nil
	}
	return []string{rel}
}

func writeGallery(cfg *galleryConfig) error {
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, filepath.FromSlash(galleryConfigFile)), append(raw, '\n'), 0o644)
}

// oneLine — подпись в одну строку и с потолком длины.
func oneLine(s string, max int) string {
	return cutRunes(strings.Join(strings.Fields(s), " "), max)
}
