//go:generate goversioninfo -icon=./static/favicon.ico -manifest=./V2RayWebUI.exe.manifest
package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/andreyvit/systemproxy"
	"github.com/getlantern/systray"
	"github.com/go-ini/ini"
)

const configFileName = "V2RayWebUI.ini"

var (
	cfg                *ini.File
	listenAddress      string
	listenPort         int
	openBrowser        bool
	setSystemProxyPort int

	v2rayIsRunning bool
	logFileName    string
	cmd            *exec.Cmd
)

func main() {
	listenAddress = "0.0.0.0"
	listenPort = 2100
	openBrowser = true
	setSystemProxyPort = 2180
	var err error
	cfg, err = ini.Load(configFileName)
	if err != nil {
		fmt.Printf("Fail to read config file: %v", err)
	} else {
		listenAddress = cfg.Section("").Key("listen").Value()
		listenPort = cfg.Section("").Key("port").MustInt(2100)
		openBrowser = cfg.Section("").Key("open_browser").MustBool(true)
		setSystemProxyPort = cfg.Section("").Key("set_system_proxy_port").MustInt(2180)
	}
	v2rayIsRunning = false
	logFileName = filepath.Join(os.TempDir(), "V2RayWebUI.log")
	fmt.Println("V2Ray Web UI started, listening on port " + strconv.Itoa(listenPort))
	logFile, _ := os.Create(logFileName) // create log file
	os.Stdout = logFile

	fmt.Println("Log path: ", logFileName)
	systray.Run(onReady, onExit)
}

func onReady() {
	icon, _ := ioutil.ReadFile("./static/favicon.ico")
	systray.SetIcon(icon)
	systray.SetTitle("V2Ray Web UI")
	openWebMenuItem := systray.AddMenuItem("打开 V2Ray Web UI", "打开 V2Ray Web UI")
	autoOpenMenuItem := systray.AddMenuItem("启动时自动打开浏览器", "启动时自动打开浏览器")
	if openBrowser {
		autoOpenMenuItem.Check()
	}
	setSystemProxyMenuItem := systray.AddMenuItem("设置为默认系统代理", "设置为默认系统代理")
	proxySetting, err := systemproxy.Get()
	if err != nil {
		fmt.Printf("error getting system proxy %v", err)
	} else {
		if proxySetting.Enabled {
			setSystemProxyMenuItem.Check()
		}
	}
	mQuit := systray.AddMenuItem("退出", "退出程序")
	go func() {
		for {
			select {
			case <-openWebMenuItem.ClickedCh:
				openbrowser()
			case <-autoOpenMenuItem.ClickedCh:
				if autoOpenMenuItem.Checked() {
					autoOpenMenuItem.Uncheck()
					if openBrowser {
						openBrowser = false
						cfg.Section("").Key("open_browser").SetValue("false")
						cfg.SaveTo(configFileName)
					}
				} else {
					autoOpenMenuItem.Check()
					if !openBrowser {
						openBrowser = true
						cfg.Section("").Key("open_browser").SetValue("true")
						cfg.SaveTo(configFileName)
					}
				}
			case <-setSystemProxyMenuItem.ClickedCh:
				if setSystemProxyMenuItem.Checked() {
					setSystemProxyMenuItem.Uncheck()
					setSystemProxyMenuItem.SetTooltip("当前未设置为默认系统代理")
					systemproxy.Set(systemproxy.Settings{
						Enabled: false,
					})
				} else {
					setSystemProxyMenuItem.Check()
					setSystemProxyMenuItem.SetTooltip("当前设置为默认系统代理")
					systemproxy.Set(systemproxy.Settings{
						Enabled:       true,
						DefaultServer: "127.0.0.1:" + strconv.Itoa(setSystemProxyPort),
					})
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()

	startV2Ray()

	// handler all request start from "/"
	http.HandleFunc("/", handler)

	if openBrowser {
		go func() {
			openbrowser()
		}()
	}
	// start HTTP server in socket
	http.ListenAndServe(":"+strconv.Itoa(listenPort), nil)
}

func onExit() {
	stopV2Ray()
}

func getIcon(s string) []byte {
	b, err := ioutil.ReadFile(s)
	if err != nil {
		fmt.Print(err)
	}
	return b
}

func startV2Ray() error {
	if v2rayIsRunning {
		return errors.New("V2Ray 已经在运行")
	}
	var v2rayExec string
	switch runtime.GOOS {
	case "windows":
		v2rayExec = "./v2ray/wv2ray.exe"
	default:
		v2rayExec = "./v2ray/v2ray"
	}
	v2rayIsRunning = true
	systray.SetTooltip("V2Ray 正在运行")
	cmd = exec.Command(v2rayExec)
	stdoutIn, _ := cmd.StdoutPipe()
	stderrIn, _ := cmd.StderrPipe()
	err := cmd.Start()
	if err != nil {
		return err
	}
	go func() {
		err := cmd.Wait()
		v2rayIsRunning = false
		systray.SetTooltip("V2Ray 已经停止")
		fmt.Println("V2Ray exit: " + err.Error())
	}()
	go func() {
		if !v2rayIsRunning {
			return
		}
		copyAndCapture(stdoutIn)
	}()
	go func() {
		if !v2rayIsRunning {
			return
		}
		copyAndCapture(stderrIn)
	}()
	return nil
}

func stopV2Ray() error {
	if !v2rayIsRunning {
		return errors.New("V2Ray 已经停止")
	}
	v2rayIsRunning = false
	systray.SetTooltip("V2Ray 已经停止")
	if err := cmd.Process.Kill(); err != nil {
		return err
	}
	return nil
}

func openbrowser() {
	url := "http://localhost:" + strconv.Itoa(listenPort) + "/"
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Fatal(err)
	}
}

func copyAndCapture(r io.Reader) ([]byte, error) {
	buf := make([]byte, 1024, 1024)
	for {
		n, _ := r.Read(buf[:])
		if n > 0 {
			d := buf[:n]
			os.Stdout.Write(d)
		}
	}
}

// Handle HTTP request to either static file server or REST server (URL start with "api/")
func handler(w http.ResponseWriter, r *http.Request) {
	//remove first "/" character
	urlPath := r.URL.Path[1:]

	//if start with "api/" direct to REST handler
	if strings.HasPrefix(urlPath, "api/") {
		trimmedURL := urlPath[4:]
		routePath(w, r, trimmedURL)
	} else {
		staticFilePath := "./static/"
		http.ServeFile(w, r, staticFilePath+urlPath)
	}
}

//handle dynamic HTTP user requset
func routePath(w http.ResponseWriter, r *http.Request, trimURL string) {
	if strings.HasPrefix(trimURL, "getConfig") {
		// example URL: localhost:2100/api/getConfig
		// try read file
		data, err := ioutil.ReadFile("./v2ray/config.json")
		if err != nil {
			handleAPIErrorCode(500, "获取文件失败", w)
		} else {
			w.Header().Set("Content-Type", "text/plain")
			w.Write(data)
		}
	} else if strings.HasPrefix(trimURL, "saveConfig") {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			// show error page if failed to read file
			handleAPIErrorCode(500, "获取数据失败", w)
		} else {
			if _, err := os.Stat("./v2ray"); os.IsNotExist(err) {
				os.Mkdir("./v2ray", 0644)
			}
			err := ioutil.WriteFile("./v2ray/config.json", body, 0644)
			if err != nil {
				// show error page if failed to read file
				handleAPIErrorCode(500, "写入文件失败", w)
			}
		}
	} else if strings.HasPrefix(trimURL, "getLog") {
		data, err := ioutil.ReadFile(logFileName)
		if err != nil {
			handleAPIErrorCode(500, "获取日志文件失败", w)
		} else {
			w.Header().Set("Content-Type", "text/plain")
			w.Write(data)
		}
	} else if strings.HasPrefix(trimURL, "getStatus") {
		w.Header().Set("Content-Type", "text/plain")
		if v2rayIsRunning {
			w.Write([]byte("running"))
		} else {
			w.Write([]byte("exit"))
		}
	} else if strings.HasPrefix(trimURL, "start") {
		err := startV2Ray()
		if err != nil {
			handleAPIErrorCode(500, err.Error(), w)
		}
	} else if strings.HasPrefix(trimURL, "stop") {
		err := stopV2Ray()
		if err != nil {
			handleAPIErrorCode(500, err.Error(), w)
		}
	} else if strings.HasPrefix(trimURL, "restart") {
		stopV2Ray()
		startV2Ray()
	} else {
		// show error code 404 not found
		handleErrorCode(404, "找不到路径", w)
	}
}

// Generate error page
func handleErrorCode(errorCode int, description string, w http.ResponseWriter) {
	w.WriteHeader(errorCode)                    // set HTTP status code (example 404, 500)
	w.Header().Set("Content-Type", "text/html") // clarify return type (MIME)
	w.Write([]byte(fmt.Sprintf(
		"<html><body><h1>Error %d</h1><p>%s</p></body></html>",
		errorCode,
		description)))
}

func handleAPIErrorCode(errorCode int, description string, w http.ResponseWriter) {
	w.WriteHeader(errorCode)
	w.Write([]byte(description))
}
