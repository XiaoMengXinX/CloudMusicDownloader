package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	dl "github.com/XiaoMengXinX/CloudMusicDownloader/downloader"
	"github.com/XiaoMengXinX/Music163Api-Go/api"
	"github.com/XiaoMengXinX/Music163Api-Go/types"
	"github.com/XiaoMengXinX/Music163Api-Go/utils"
	"github.com/nfnt/resize"
	log "github.com/sirupsen/logrus"
	"github.com/vbauerster/mpb/v5"
	"image"
	"image/jpeg"
	// 解析 gif 图片信息
	_ "image/gif"
	// 解析 png 图片信息
	_ "image/png"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
	"time"
)

//var _163key = flag.String("dec", "", "Decode 163 key")
var _playlistID = flag.Int("p", 0, "要批量下载的歌单ID (必填)")
var _MUSIC_U = flag.String("MUSIC_U", "", "账号cookie中的MUSIC_U (必填)")
var _offset = flag.Int("o", 0, "歌单歌曲偏移量 (选填, 即从第几个歌曲开始下载)")
var _output = flag.String("path", "", "音乐存放目录 (选填, 默认为./music)")
var _concurrent = flag.Int("c", 4, "并发下载任务数量 (选填, 默认为4. 并发任务越多占用内存越大)")
var _check = flag.Bool("check", false, "扫描目录下的歌曲")
var _mp3 = flag.Bool("mp3", false, "强制下载mp3格式")

var d *downloader
var p *mpb.Progress
var processNum int
var downloadNum int
var failedNum int

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
	if *_offset != 0 {
		*_offset = *_offset - 1
	}
	if *_output != "" {
		MusicDir = *_output
	}

	logDir := fmt.Sprintf("./downloader.log")
	logFile, err := os.OpenFile(logDir, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Error(err)
	}
	output := io.MultiWriter(logFile)
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
		fmt.Println("参数缺失，请检查是否正确传入 歌单ID 和 MUSIC_U")
		log.Fatal("参数缺失，请检查是否正确传入 歌单ID 和 MUSIC_U")
	}
}

func main() {
	/*
		if *_163key != "" {
			fmt.Println(dl.Decrypt163key(*_163key))
			os.Exit(0)
		}
	*/

	var playListDetail types.PlaylistDetailData
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
		fmt.Println("获取账号登录状态失败，请检查 MUSIC_U 是否有效")
		log.Fatal("获取账号登录状态失败，请检查 MUSIC_U 是否有效")
	} else {
		fmt.Printf("[%s] 获取账号登录状态成功\n", loginStat.Profile.Nickname)
		log.Printf("[%s] 获取账号登录状态成功", loginStat.Profile.Nickname)
		time.Sleep(time.Duration(2) * time.Second)
	}

	if *_check {
		tempPlaylist := checkMusic(data)
		playListDetail = tempPlaylist
	}

	checkPathExists(MusicDir)
	checkPathExists(PicDir)
	d = newDownloader()
	d.Concurrent = *_concurrent
	d.Pool = make(chan *resource, d.Concurrent)
	p = mpb.New(mpb.WithWaitGroup(d.WG))

	if len(playListDetail.Playlist.TrackIds) == 0 {
		tempPlaylist, err := api.GetPlaylistDetail(data, *_playlistID)
		if err != nil {
			fmt.Println("获取歌单信息失败")
			log.Fatal("获取歌单信息失败")
		}
		playListDetail = tempPlaylist
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
			time.Sleep(time.Duration(5) * time.Second)
		}
	}()

	i := 0
	for true {
		for ; i*2 < len(d.resources)-1; i++ {
			d.WG.Add(1)
			a := i * 2
			go func() {
				err := start(d, a)
				if err != nil {
					log.Errorln(err)
				}
			}()
			time.Sleep(time.Duration(100) * time.Millisecond)
		}
		if processNum == len(playListDetail.Playlist.TrackIds) && i*2 == len(d.resources) {
			break
		}
		time.Sleep(time.Duration(100) * time.Millisecond)
	}
	p.Wait()
	d.WG.Wait()
	log.Printf("下载完成, %d 首歌曲下载成功, %d 首歌曲下载失败", downloadNum, failedNum)
	fmt.Printf("下载完成, %d 首歌曲下载成功, %d 首歌曲下载失败\n", downloadNum, failedNum)
}

func start(d *downloader, a int) (err error) {
	defer d.WG.Done()
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
	format := strings.Replace(path.Ext(d.resources[a].ReadName), ".", "", -1)

	picPath := d.resources[a+1].TargetDir + "/" + d.resources[a+1].Filename
	picStat, _ := os.Stat(picPath)
	if picStat.Size() > 2*1024*1024 {
		resizePath, err := resizePic(picPath)
		if err == nil {
			picPath = resizePath
		} else {
			log.Errorln(err)
		}
	}

	if fileExist(d.resources[a].TargetDir+"/"+d.resources[a].Filename) && fileExist(d.resources[a+1].TargetDir+"/"+d.resources[a+1].Filename) {
		switch format {
		case "flac":
			err := dl.AddFlacId3v2(d.resources[a].TargetDir+"/"+d.resources[a].Filename, picPath, marker, d.resources[a].SongDetail)
			if err != nil {
				return err
			}
		case "mp3":
			err := dl.AddMp3Id3v2(d.resources[a].TargetDir+"/"+d.resources[a].Filename, picPath, marker, d.resources[a].SongDetail)
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
		} else {
			failedNum++
			log.Printf("[%s] 下载失败", d.resources[a].ReadName)
			return
		}
	}
	err = os.Rename(d.resources[a].TargetDir+"/"+d.resources[a].Filename, d.resources[a].TargetDir+"/"+d.resources[a].ReadName)
	if err != nil {
		failedNum++
		return err
	}
	log.Printf("[%s] 下载成功", d.resources[a].ReadName)
	runtime.GC()
	return
}

func getPlaylistMusic(data utils.RequestData, playListDetail types.PlaylistDetailData) (err error) {
	var songID int
	var replacer = strings.NewReplacer("/", " ", "?", " ", "*", " ", ":", " ", "|", " ", "\\", " ", "<", " ", ">", " ", "\"", " ")
	for processNum = *_offset; processNum < len(playListDetail.Playlist.TrackIds); {
		if len(d.resources)/2-downloadNum > 10 {
			continue
		}

		songID = playListDetail.Playlist.TrackIds[processNum].Id
		songURLConfig := api.SongURLConfig{Ids: []int{songID}}
		if *_mp3 {
			songURLConfig.Level = "higher"
		}
		b := api.NewBatch(
			api.BatchAPI{
				Key:  api.SongDetailAPI,
				Json: api.CreateSongDetailReqJson([]int{songID}),
			}, api.BatchAPI{
				Key:  api.SongUrlAPI,
				Json: api.CreateSongURLJson(songURLConfig),
			},
		).Do(data)
		if b.Error != nil {
			return err
		}
		var songDetail types.BatchSongDetailData
		var songUrl types.BatchSongURLData
		_ = json.Unmarshal([]byte(b.Result), &songDetail)
		_ = json.Unmarshal([]byte(b.Result), &songUrl)

		if songUrl.Api.Data[0].Url != "" && songDetail.Api.Songs[0].Al.PicUrl != "" {
			d.appendResource(
				MusicDir,
				replacer.Replace(fmt.Sprintf("%s - %s.%s.download", dl.ParseArtist(songDetail.Api.Songs[0]), songDetail.Api.Songs[0].Name, songUrl.Api.Data[0].Type)),
				replacer.Replace(fmt.Sprintf("%s - %s.%s", dl.ParseArtist(songDetail.Api.Songs[0]), songDetail.Api.Songs[0].Name, songUrl.Api.Data[0].Type)),
				songUrl.Api.Data[0].Url,
				songDetail.Api.Songs[0].Name,
				songUrl.Api.Data[0].Type,
				songDetail.Api.Songs[0],
				songUrl.Api.Data[0],
				false)
			d.appendResource(
				PicDir,
				fmt.Sprintf("%d%s", songDetail.Api.Songs[0].Id, path.Ext(songDetail.Api.Songs[0].Al.PicUrl)),
				fmt.Sprintf("%d%s", songDetail.Api.Songs[0].Id, path.Ext(songDetail.Api.Songs[0].Al.PicUrl)),
				songDetail.Api.Songs[0].Al.PicUrl,
				fmt.Sprintf("%d%s", songDetail.Api.Songs[0].Id, path.Ext(songDetail.Api.Songs[0].Al.PicUrl)),
				path.Ext(songDetail.Api.Songs[0].Al.PicUrl),
				types.SongDetailData{},
				types.SongURLData{},
				true)
		} else {
			failedNum++
		}
		processNum++

		time.Sleep(time.Duration(50) * time.Millisecond)
	}
	return
}

//goland:noinspection GoNilness
func checkMusic(data utils.RequestData) types.PlaylistDetailData {
	var musicIDs []int
	var processFile = 1
	fmt.Printf("正在读取 %s 目录下文件", MusicDir)
	log.Printf("正在读取 %s 目录下文件", MusicDir)
	files, _ := ioutil.ReadDir(MusicDir)
	allFile := len(files)
	for _, f := range files {
		fmt.Printf("\r正在扫描 %s 目录下文件 (%d/%d)", MusicDir, processFile, allFile)
		processFile++
		if strings.Contains(f.Name(), ".mp3") && !strings.Contains(f.Name(), ".tmp") && !strings.Contains(f.Name(), ".download") {
			marker, err := dl.ReadMp3Key(fmt.Sprintf("%s/%s", MusicDir, f.Name()))
			if err != nil {
				log.Errorln(err)
			}
			musicIDs = append(musicIDs, marker.MusicId)
			continue
		}
		if strings.Contains(f.Name(), ".flac") && !strings.Contains(f.Name(), ".tmp") && !strings.Contains(f.Name(), ".download") {
			marker, err := dl.ReadFlacKey(fmt.Sprintf("%s/%s", MusicDir, f.Name()))
			if err != nil {
				log.Errorln(err)
			}
			musicIDs = append(musicIDs, marker.MusicId)
			continue
		}
	}
	fmt.Println()

	playListDetail, err := api.GetPlaylistDetail(data, *_playlistID)
	if err != nil {
		fmt.Println("获取歌单信息失败")
		log.Fatal("获取歌单信息失败")
	}
	var plMusicIDs []int
	for _, n := range playListDetail.Playlist.TrackIds {
		plMusicIDs = append(plMusicIDs, n.Id)
	}
	inMusicIDs := intersect(musicIDs, plMusicIDs)
	outMusicIDs := difference(plMusicIDs, musicIDs)
	fmt.Printf("歌单 [%s] 歌曲共 %d 首, 本地歌曲共 %d 首, 歌单内本地歌曲共 %d 首, 未下载歌曲共 %d 首\n", playListDetail.Playlist.Name, len(plMusicIDs), len(musicIDs), len(inMusicIDs), len(outMusicIDs))

	if len(outMusicIDs) == 0 {
		os.Exit(0)
	}

	fmt.Printf("按 [ENTER] 继续下载歌单外歌曲, 输入任意字符退出\n")

	inputReader := bufio.NewReader(os.Stdin)
	input, _ := inputReader.ReadString('\n')
	fmt.Printf("%+v", input)
	if input != "\n" && input != "\r\n" {
		os.Exit(0)
	}

	var tempPlaylist types.PlaylistDetailData
	for _, id := range outMusicIDs {
		var trackIDs struct {
			Id         int         `json:"id"`
			V          int         `json:"v"`
			T          int         `json:"t"`
			At         int         `json:"at"`
			Alg        interface{} `json:"alg"`
			Uid        int         `json:"uid"`
			RcmdReason string      `json:"rcmdReason"`
		}
		trackIDs.Id = id
		tempPlaylist.Playlist.TrackIds = append(tempPlaylist.Playlist.TrackIds, trackIDs)
	}
	return tempPlaylist
}

func resizePic(picPath string) (resizePath string, err error) {
	file, err := os.Open(picPath)
	defer func(file *os.File) {
		e := file.Close()
		if e != nil {
			err = e
		}
	}(file)
	if err != nil {
		return resizePath, err
	}
	img, _, err := image.Decode(file)
	if err != nil {
		return resizePath, err
	}

	m := resize.Resize(0, 800, img, resize.Lanczos3)

	fileDir := path.Dir(picPath)
	fileNameWithSuffix := path.Base(picPath)
	fileSuffix := path.Ext(fileNameWithSuffix)
	fileName := strings.TrimSuffix(fileNameWithSuffix, fileSuffix)
	resizePath = fmt.Sprintf("%s/%s.resize%s", fileDir, fileName, fileSuffix)

	out, err := os.Create(resizePath)
	if err != nil {
		return "", err
	}
	defer func(out *os.File) {
		e := out.Close()
		if e != nil {
			err = e
		}
	}(out)

	err = jpeg.Encode(out, m, nil)
	if err != nil {
		return "", err
	}
	return resizePath, err
}

func intersect(slice1, slice2 []int) []int {
	m := make(map[int]int)
	nn := make([]int, 0)
	for _, v := range slice1 {
		m[v]++
	}
	for _, v := range slice2 {
		times, _ := m[v]
		if times == 1 {
			nn = append(nn, v)
		}
	}
	return nn
}

func difference(slice1, slice2 []int) []int {
	m := make(map[int]int)
	nn := make([]int, 0)
	inter := intersect(slice1, slice2)
	for _, v := range inter {
		m[v]++
	}
	for _, value := range slice1 {
		times, _ := m[value]
		if times == 0 {
			nn = append(nn, value)
		}
	}
	return nn
}
