package downloader

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/XiaoMengXinX/Music163Api-Go/types"
	"github.com/XiaoMengXinX/Music163Api-Go/utils"
	"github.com/bogem/id3v2"
	"github.com/go-flac/flacpicture"
	"github.com/go-flac/flacvorbis"
	"github.com/go-flac/go-flac"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"strings"
)

// AddMp3Id3v2 添加 mp3 的 id3v2 信息
func AddMp3Id3v2(musicPath, picPath, musicMarker string, songDetail types.SongDetailData) error {
	musicTag, _ := id3v2.Open(musicPath, id3v2.Options{Parse: false})
	defer func(musicTag *id3v2.Tag) {
		err := musicTag.Close()
		if err != nil {
			log.Errorln(err)
		}
	}(musicTag)
	musicTag.SetDefaultEncoding(id3v2.EncodingUTF8)
	musicTag.SetTitle(songDetail.Name)
	musicTag.SetArtist(ParseArtist(songDetail))
	if songDetail.Al.Name != "" {
		musicTag.SetAlbum(songDetail.Al.Name)
	}
	comment := id3v2.CommentFrame{
		Encoding:    id3v2.EncodingUTF8,
		Language:    "eng",
		Description: "",
		Text:        musicMarker,
	}
	musicTag.AddCommentFrame(comment)
	if picPath != "" {
		artwork, err := ioutil.ReadFile(picPath)
		if err != nil {
			return fmt.Errorf("Error while reading AlbumPic: %v ", err)
		}
		var mime string
		fileCode := bytesToHexString(artwork[:32])
		if strings.HasPrefix(fileCode, "ffd8ffe000104a464946") {
			mime = "image/jpeg"
		}
		if strings.HasPrefix(fileCode, "89504e470d0a1a0a0000") {
			mime = "image/png"
		}
		if mime != "" {
			pic := id3v2.PictureFrame{
				Encoding:    id3v2.EncodingISO,
				MimeType:    mime,
				PictureType: id3v2.PTFrontCover,
				Description: "Front cover",
				Picture:     artwork,
			}
			musicTag.AddAttachedPicture(pic)
		}
	}
	if err := musicTag.Save(); err != nil {
		return fmt.Errorf("Error: %v ", err)
	}
	return nil
}

// AddFlacId3v2 添加 flac 的 id3v2 信息
func AddFlacId3v2(musicPath, picPath, musicMarker string, songDetail types.SongDetailData) error {
	file, err := flac.ParseFile(musicPath)
	if err != nil {
		return err
	}
	cmts := flacvorbis.New()

	if picPath != "" {
		artwork, err := ioutil.ReadFile(picPath)
		if err != nil {
			return err
		}
		var mime string
		fileCode := bytesToHexString(artwork)
		if strings.HasPrefix(fileCode, "ffd8ffe000104a464946") {
			mime = "image/jpeg"
		}
		if strings.HasPrefix(fileCode, "89504e470d0a1a0a0000") {
			mime = "image/png"
		}
		if mime != "" {
			picture, err := flacpicture.NewFromImageData(flacpicture.PictureTypeFrontCover, "Front cover", artwork, mime)
			if err == nil {
				picturemeta := picture.Marshal()
				file.Meta = append(file.Meta, &picturemeta)
			}
		}
	}

	_ = cmts.Add(flacvorbis.FIELD_TITLE, songDetail.Name)
	_ = cmts.Add(flacvorbis.FIELD_ARTIST, ParseArtist(songDetail))
	if songDetail.Al.Name != "" {
		_ = cmts.Add(flacvorbis.FIELD_ALBUM, songDetail.Al.Name)
	}
	_ = cmts.Add(flacvorbis.FIELD_DESCRIPTION, musicMarker)

	block := cmts.Marshal()
	var idx = -1
	for i, m := range file.Meta {
		if m.Type == flac.VorbisComment {
			idx = i
			break
		}
	}
	if idx == -1 {
		file.Meta = append(file.Meta, &block)
	} else {
		file.Meta[idx] = &block
	}

	err = file.Save(musicPath)
	if err != nil {
		return err
	}

	return err
}

// CreateMarker 格式化 marker 信息
func CreateMarker(songDetail types.SongDetailData, songUrl types.SongURLData) (markerText string, err error) {
	var artists [][]interface{}
	for _, j := range songDetail.Ar {
		var artist []interface{}
		artist = make([]interface{}, 2)
		artist[0] = j.Name
		artist[1] = j.Id
		artists = append(artists, artist)
	}
	marker := MarkerData{
		MusicId:       songDetail.Id,
		MusicName:     songDetail.Name,
		Artist:        artists,
		AlbumId:       songDetail.Al.Id,
		Album:         songDetail.Al.Name,
		AlbumPicDocId: songDetail.Al.PicStr,
		AlbumPic:      songDetail.Al.PicUrl,
		Bitrate:       songUrl.Br,
		Mp3DocId:      songUrl.Md5,
		Duration:      songDetail.Dt,
		MvId:          songDetail.Mv,
		Alias:         songDetail.Alia,
		Format:        songUrl.Type,
	}
	markerJson, err := json.Marshal(marker)
	if err != nil {
		return "", err
	}
	decryptedMarker := base64.StdEncoding.EncodeToString(utils.MarkerAesEncryptECB(fmt.Sprintf("music:%s", string(markerJson))))
	markerText = fmt.Sprintf("163 key(Don't modify):%s", decryptedMarker)
	return markerText, fmt.Errorf("SongDetail 与 SongUrl 元素数量不相等")
}
