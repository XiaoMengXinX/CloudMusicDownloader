package main

import (
	"encoding/json"
	"flag"
	"fmt"
	dl "github.com/XiaoMengXinX/CloudMusicDownloader/downloader"
	"github.com/XiaoMengXinX/Music163Api-Go/api"
	"github.com/XiaoMengXinX/Music163Api-Go/types"
	"github.com/XiaoMengXinX/Music163Api-Go/utils"
	log "github.com/sirupsen/logrus"
	"github.com/vbauerster/mpb/v5"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

// var _163key = flag.String("dec", "", "Decode 163 key")
var _playlistID = flag.Int("p", 0, "要批量下载的歌单ID （必填）")
var _MUSIC_U = flag.String("MUSIC_U", "", "账号cookie中的MUSIC_U （必填）")
var _concurrent = flag.Int("c", 4, "并发下载任务数量 （选填，默认为4）")

var d *downloader
var p *mpb.Progress
var processNum int
var downloadNum int

// LogFormatter 自定义 log 格式
type LogFormatter struct{}

// Format 自定义 log 格式
func (s *LogFormatter) Format(entry *log.Entry) ([]byte, error) {
	timestamp := time.Now().Local().Format("2006/01/02 15:04:05")
	var msg string
	msg = fmt.Sprintf("%s [%s] %s (%s:%d)\n", timestamp, strings.ToUpper(entry.Level.String()), entry.Message, path.Base(entry.Caller.File), entry.Caller.Line)
	return []byte(msg), nil
}

func init() {
	flag.Parse()

	logFile := fmt.Sprintf("./downloader.log")
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Error(err)
	}
	output := io.MultiWriter(file, os.Stdout)
	log.SetOutput(output)
	log.SetFormatter(&log.TextFormatter{
		DisableColors:          false,
		FullTimestamp:          true,
		DisableLevelTruncation: true,
		PadLevelText:           true,
	})
	log.SetFormatter(new(LogFormatter))
	log.SetReportCaller(true)

	if *_playlistID == 0 || *_MUSIC_U == "" {
		log.Fatal("参数缺失，请检查是否正确传入 歌单ID 和 MUSIC_U")
	}

	checkPathExists(MusicDir)
	checkPathExists(PicDir)

	d = newDownloader()
	d.Concurrent = *_concurrent
	d.Pool = make(chan *resource, d.Concurrent)
	p = mpb.New(mpb.WithWaitGroup(d.WG))
}

func main() {
	/*
		if *_163key != "" {
			fmt.Println(dl.Decrypt163key(*_163key))
			os.Exit(0)
		}
	*/

	data := utils.RequestData{
		Cookies: []*http.Cookie{
			{
				Name:  "MUSIC_U",
				Value: *_MUSIC_U,
			},
		},
	}

	loginStat, _ := api.GetLoginStatus(data)
	if loginStat.Account.Id == 0 {
		log.Fatal("获取账号登录状态失败，请检查 MUSIC_U 是否有效")
	} else {
		log.Printf("[%s] 获取账号登录状态成功", loginStat.Profile.Nickname)
		time.Sleep(time.Duration(2) * time.Second)
	}

	playListDetail, err := api.GetPlaylistDetail(data, *_playlistID)
	if err != nil {
		log.Fatal("获取歌单信息失败")
	}
	go func() {
		err := getPlaylistMusic(data, playListDetail)
		if err != nil {
			log.Errorln(err)
		}
	}()

	go func() {
		for true {
			fmt.Printf("\x1bc")
			time.Sleep(time.Duration(2) * time.Second)
		}
	}()

	i := 0
	for true {
		if processNum == len(playListDetail.Playlist.TrackIds) && (i+1)*2 == len(d.resources) {
			break
		}
		for ; i*2 < len(d.resources)-1; i++ {
			d.WG.Add(1)
			a := i * 2
			go func() {
				err := start(d, a)
				if err != nil {
					log.Errorln(err)
				}
			}()
		}
	}
	p.Wait()
	d.WG.Wait()
}

func start(d *downloader, a int) (err error) {
	if !fileExist(fmt.Sprintf("%s%s", d.resources[a].SongDetail.Al.PicStr, path.Ext(d.resources[a].SongDetail.Al.PicUrl))) {
		err = d.download(d.resources[a+1], p)
		if err != nil {
			log.Errorln(err)
		}
	}
	err = d.download(d.resources[a], p)
	if err != nil {
		log.Errorln(err)
	}
	downloadNum = downloadNum + 1
	marker, _ := dl.CreateMarker(d.resources[a].SongDetail, d.resources[a].SongURL)
	format := strings.Replace(path.Ext(d.resources[a].Filename), ".", "", -1)
	if fileExist(d.resources[a].TargetDir+"/"+d.resources[a].Filename) && fileExist(d.resources[a+1].TargetDir+"/"+d.resources[a+1].Filename) {
		switch format {
		case "flac":
			err := dl.AddFlacId3v2(d.resources[a].TargetDir+"/"+d.resources[a].Filename, d.resources[a+1].TargetDir+"/"+d.resources[a+1].Filename, marker, d.resources[a].SongDetail)
			if err != nil {
				return err
			}
		case "mp3":
			err := dl.AddMp3Id3v2(d.resources[a].TargetDir+"/"+d.resources[a].Filename, d.resources[a+1].TargetDir+"/"+d.resources[a+1].Filename, marker, d.resources[a].SongDetail)
			if err != nil {
				return err
			}
		}
	} else {
		if fileExist(d.resources[a].TargetDir + "/" + d.resources[a].Filename) {
			switch format {
			case "flac":
				err := dl.AddFlacId3v2(d.resources[a].TargetDir+"/"+d.resources[a].Filename, "", marker, d.resources[a].SongDetail)
				if err != nil {
					return err
				}
			case "mp3":
				err := dl.AddMp3Id3v2(d.resources[a].TargetDir+"/"+d.resources[a].Filename, "", marker, d.resources[a].SongDetail)
				if err != nil {
					return err
				}
			}
		}
	}
	d.WG.Done()
	return
}

func getPlaylistMusic(data utils.RequestData, playListDetail types.PlaylistDetailData) (err error) {
	var songID int
	var replacer = strings.NewReplacer("/", " ", "?", " ", "*", " ", ":", " ", "|", " ", "\\", " ", "<", " ", ">", " ", "\"", " ")
	for i := -1; i < len(playListDetail.Playlist.TrackIds)-1; {
		if len(d.resources)/2-downloadNum > 10 {
			continue
		} else {
			i++
			processNum = i + 1
		}
		songID = playListDetail.Playlist.TrackIds[i].Id
		var b api.Batch
		b.Init()
		b.Add(
			api.BatchAPI{
				Key:  api.SongDetail,
				Json: api.CreateSongDetailReqJson([]int{songID}),
			}, api.BatchAPI{
				Key:  api.SongURL,
				Json: api.CreateSongURLJson(api.SongURLConfig{Ids: []int{songID}}),
			},
		)
		result, _, err := b.Do(data)
		if err != nil {
			return err
		}
		var songDetail types.BatchSongDetailData
		var songUrl types.BatchSongURLData
		_ = json.Unmarshal([]byte(result), &songDetail)
		_ = json.Unmarshal([]byte(result), &songUrl)

		if songUrl.Api.Data[0].Url != "" && songDetail.Api.Songs[0].Al.PicUrl != "" {
			d.appendResource(
				MusicDir,
				replacer.Replace(fmt.Sprintf("%s - %s.%s", dl.ParseArtist(songDetail.Api.Songs[0]), songDetail.Api.Songs[0].Name, songUrl.Api.Data[0].Type)),
				songUrl.Api.Data[0].Url,
				songDetail.Api.Songs[0].Name,
				songUrl.Api.Data[0].Type,
				songDetail.Api.Songs[0],
				songUrl.Api.Data[0],
				false)
			d.appendResource(
				PicDir,
				fmt.Sprintf("%s%s", songDetail.Api.Songs[0].Al.PicStr, path.Ext(songDetail.Api.Songs[0].Al.PicUrl)),
				songDetail.Api.Songs[0].Al.PicUrl,
				songDetail.Api.Songs[0].Al.PicStr,
				path.Ext(songDetail.Api.Songs[0].Al.PicUrl),
				types.SongDetailData{},
				types.SongURLData{},
				true)
		}
	}
	return
}
