package main

import (
	"bufio"
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var cfgFile string

func init() {
	pflag.StringVar(&cfgFile, "config", "", "config file, default: $HOME/.enview.toml")
	pflag.String("addr", ":8080", "address to listen on")
	pflag.String("static", "./public", "directory to serve static files from")
	viper.SetDefault("enview.log.dir", "./logs")
	viper.BindPFlag("httpd.address", pflag.Lookup("addr"))
	viper.BindPFlag("httpd.static.dir", pflag.Lookup("static"))
	viper.SetDefault("httpd.template.dir", "./public/templates")
}

func initializeConfig(cfgFile string) error {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName(".enview")
		viper.SetConfigType("toml")
		viper.AddConfigPath("$HOME")
	}
	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("error loading config file: %s", err)
	}
	return nil
}

func main() {
	pflag.Parse()
	if err := initializeConfig(cfgFile); err != nil {
		log.Fatalf("error initializing config: %v", err)
	}

	http.Handle("/", http.HandlerFunc(ViewPath))
	http.Handle("/favicon.ico", http.HandlerFunc(http.NotFound))
	http.Handle("/search/", http.StripPrefix("/search", http.HandlerFunc(SearchPath)))

	static := http.StripPrefix("/static", http.FileServer(http.Dir(viper.GetString("httpd.static.dir"))))
	http.Handle("/static/", static)

	addr := viper.GetString("httpd.address")
	log.Println("listening on", addr)
	log.Fatalln(http.ListenAndServe(addr, nil))
}

func SearchPath(w http.ResponseWriter, r *http.Request) {
	crumbs := breadcrumbs(r)
	p := logPath(r.URL.Path)
	query := r.FormValue("q")
	results, err := grep(query, p, r.URL.Path)
	if err != nil {
		internalServerError(w, "grep", err)
	}
	sort.Sort(sort.Reverse(grepResultSort(results)))
	renderView(w, r, "search", nil, map[string]interface{}{
		"Crumbs":  crumbs,
		"Query":   query,
		"Results": results,
	})
}

func ViewPath(w http.ResponseWriter, r *http.Request) {
	crumbs := breadcrumbs(r)
	p := logPath(r.URL.Path)
	if isDir, err := isDirectory(p); err != nil {
		internalServerError(w, "isDirectory", err)
		return
	} else if isDir {
		dirs, files, err := readDir(p, r.URL.Path)
		if err != nil {
			internalServerError(w, "readDir", err)
			return
		}
		sort.Sort(sort.Reverse(directorySort(dirs)))
		sort.Sort(sort.Reverse(fileSort(files)))
		renderView(w, r, "directory", nil, map[string]interface{}{
			"Crumbs": crumbs,
			"Dirs":   dirs,
			"Files":  files,
			"Path":   r.URL.Path,
		})
	} else {
		html, err := parseLog(p)
		if err != nil {
			internalServerError(w, "parseLog", err)
		}
		renderView(w, r, "log", nil, map[string]interface{}{
			"Crumbs": crumbs,
			"HTML":   template.HTML(html),
		})
	}
}

type directory struct {
	Path string
	Name string
}

type directorySort []directory

func (a directorySort) Len() int           { return len(a) }
func (a directorySort) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a directorySort) Less(i, j int) bool { return a[i].Name < a[j].Name }

type file struct {
	Path  string
	Name  string
	Mtime time.Time
}

type fileSort []file

func (a fileSort) Len() int           { return len(a) }
func (a fileSort) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a fileSort) Less(i, j int) bool { return a[i].Mtime.Before(a[j].Mtime) }

func breadcrumbs(r *http.Request) []directory {
	xs := []directory{}
	p := r.URL.Path
	for len(p) > 1 {
		xs = append(xs, directory{
			Path: p,
			Name: path.Base(p),
		})
		p = path.Dir(p)
	}

	// https://github.com/golang/go/wiki/SliceTricks#reversing
	for i := len(xs)/2 - 1; i >= 0; i-- {
		opp := len(xs) - 1 - i
		xs[i], xs[opp] = xs[opp], xs[i]
	}

	return xs
}

type grepResult struct {
	File  string
	URL   string
	Name  string
	HTML  template.HTML
	Mtime time.Time
}

type grepResultSort []*grepResult

func (a grepResultSort) Len() int           { return len(a) }
func (a grepResultSort) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a grepResultSort) Less(i, j int) bool { return a[i].Mtime.Before(a[j].Mtime) }

func grep(query, dir, p string) (grs []*grepResult, _ error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		gr := &grepResult{
			File: filepath.Join(dir, e.Name()),
			URL:  path.Join(p, e.Name()),
			Name: e.Name(),
		}
		wg.Add(1)
		go func() {
			gr.grep(query)
			wg.Done()
		}()
		grs = append(grs, gr)
	}
	wg.Wait()
	return
}

func (gr *grepResult) HasResults() bool { return len(gr.HTML) > 0 }

func (gr *grepResult) grep(query string) {
	f, err := os.Open(gr.File)
	if err != nil {
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return
	}
	gr.Mtime = info.ModTime()

	var html string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		plain, _ := parseText(line)
		if strings.Contains(plain, query) {
			html += line + "\n"
		}
	}
	html, _ = parseHTML(html)
	gr.HTML = template.HTML(html)
}

func internalServerError(w http.ResponseWriter, tag string, err error) {
	log.Println("Internal Server Error:", tag+":", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func isDirectory(p string) (bool, error) {
	info, err := os.Stat(p)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func logPath(p string) string {
	dir := viper.GetString("enview.log.dir")
	return filepath.Join(dir, p)
}

func parseLog(p string) (string, error) {
	bs, err := ioutil.ReadFile(p)
	if err != nil {
		return "", err
	}
	return parseHTML(string(bs))
}

func readDir(dir, p string) (dirs []directory, files []file, _ error) {
	dirs = []directory{}
	files = []file{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, directory{
				Path: path.Join(p, e.Name()),
				Name: e.Name(),
			})
		} else {
			info, err := e.Info()
			if err != nil {
				return nil, nil, err
			}
			files = append(files, file{
				Path:  path.Join(p, e.Name()),
				Name:  e.Name(),
				Mtime: info.ModTime(),
			})
		}
	}
	return
}

func renderView(w http.ResponseWriter, r *http.Request, name string, helpers template.FuncMap, data interface{}) {
	funcs := template.FuncMap{
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},
	}
	for k, v := range helpers {
		funcs[k] = v
	}
	t := template.New("template").Funcs(funcs)
	tpl, err := loadTemplate("layout.html")
	if err != nil {
		internalServerError(w, "loadTemplate", err)
		return
	}
	t = template.Must(t.Parse(tpl))
	tpl, err = loadTemplate(name + ".html")
	if err != nil {
		internalServerError(w, "loadTemplate", err)
		return
	}
	t = template.Must(t.Parse(tpl))
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		// No internalServerError here because if we wrote to w in
		// ExecuteTemplate, it's already set the status code to 200, an setting
		// it again is an error.
		log.Println("ExecuteTemplate:", err)
	}
}

func loadTemplate(filename string) (string, error) {
	dir := viper.GetString("httpd.template.dir")
	filepath := path.Join(dir, filename)
	input, err := os.Open(filepath)
	if err != nil {
		return "", err
	}
	defer input.Close()
	var buf bytes.Buffer
	_, err = buf.ReadFrom(input)
	return buf.String(), err
}
