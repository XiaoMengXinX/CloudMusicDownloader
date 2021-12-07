package main

import (
	"fmt"
	"github.com/XiaoMengXinX/Music163Api-Go/types"
	log "github.com/sirupsen/logrus"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"

	"github.com/vbauerster/mpb/v5"
	"github.com/vbauerster/mpb/v5/decor"
)

var (
	// PicDir 默认封面保存目录
	PicDir = "./pic"
	// MusicDir 默认音乐下载目录
	MusicDir = "./music"
	// LyricDir 默认歌词下载目录
	LyricDir = "./lyric"
)

type resource struct {
	TargetDir          string
	Type               string
	Name               string
	Filename           string
	ReadName           string
	Url                string
	SongDetail         types.SongDetailData
	SongURL            types.SongURLData
	DisableProgressBar bool
}

type downloader struct {
	WG         *sync.WaitGroup
	Pool       chan *resource
	Concurrent int
	HttpClient http.Client
	resources  []resource
}

func newDownloader() *downloader {
	concurrent := runtime.NumCPU()
	return &downloader{
		WG:         &sync.WaitGroup{},
		Concurrent: concurrent,
	}
}

func (d *downloader) appendResource(dir, filename, readname, url, name, filetype string, SongDetail types.SongDetailData, SongURL types.SongURLData, isBarDisabled bool) {
	d.resources = append(d.resources, resource{
		TargetDir:          dir,
		Filename:           filename,
		ReadName:           readname,
		Url:                url,
		Name:               name,
		Type:               filetype,
		SongDetail:         SongDetail,
		SongURL:            SongURL,
		DisableProgressBar: isBarDisabled,
	})
}

func (d *downloader) download(resource resource, progress *mpb.Progress) error {
	d.Pool <- &resource
	finalPath := resource.TargetDir + "/" + resource.Filename

	if fileExist(finalPath + ".tmp") {
		err := os.Remove(finalPath + ".tmp")
		if err != nil {
			return err
		}
	}

	// 创建临时文件
	target, err := os.Create(finalPath + ".tmp")
	if err != nil {
		return err
	}

	// 开始下载
	req, err := http.NewRequest(http.MethodGet, resource.Url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		target.Close()
		return err
	}
	defer resp.Body.Close()
	var reader io.ReadCloser
	if resource.DisableProgressBar {
		reader = resp.Body
	} else {
		fileSize, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
		// 创建一个进度条
		bar := progress.AddBar(
			int64(fileSize),
			// 进度条前的修饰
			mpb.PrependDecorators(
				decor.Name(fmt.Sprintf("\033[32m[%s]\033[0m ", resource.Name)),
				decor.Name(resource.Type),
				decor.Name(" "),
				decor.CountersKibiByte("% .2f / % .2f"), // 已下载数量
				decor.Percentage(decor.WCSyncSpace),     // 进度百分比
			),
			// 进度条后的修饰
			mpb.AppendDecorators(
				decor.EwmaETA(decor.ET_STYLE_GO, 90),
				decor.Name(" | "),
				decor.EwmaSpeed(decor.UnitKiB, "% .2f", 60),
			),
			mpb.BarRemoveOnComplete(),
		)
		reader = bar.ProxyReader(resp.Body)
	}
	defer reader.Close()
	// 将下载的文件流拷贝到临时文件
	if _, err := io.Copy(target, reader); err != nil {
		target.Close()
		return err
	}

	// 关闭临时并修改临时文件为最终文件
	target.Close()
	if err := os.Rename(finalPath+".tmp", finalPath); err != nil {
		return err
	}
	<-d.Pool
	return nil
}

func checkPathExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		err := os.Mkdir(path, os.ModePerm)
		if err != nil {
			log.Errorf("mkdir %v failed: %v\n", path, err)
		}
		return false
	}
	log.Errorf("Error: %v\n", err)
	return false
}

func fileExist(path string) bool {
	_, err := os.Lstat(path)
	return !os.IsNotExist(err)
}
