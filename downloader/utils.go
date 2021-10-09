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
	"net/http"
	"os"
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
		Encoding:    id3v2.EncodingISO,
		Language:    "chs",
		Description: "",
		Text:        musicMarker,
	}
	musicTag.AddCommentFrame(comment)
	if picPath != "" {
		artwork, err := ioutil.ReadFile(picPath)
		if err != nil {
			return fmt.Errorf("Error while reading AlbumPic: %v ", err)
		}
		mime := http.DetectContentType(artwork[:32])
		pic := id3v2.PictureFrame{
			Encoding:    id3v2.EncodingISO,
			MimeType:    mime,
			PictureType: id3v2.PTFrontCover,
			Description: "Front cover",
			Picture:     artwork,
		}
		musicTag.AddAttachedPicture(pic)
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
	tag := flacvorbis.New()

	if picPath != "" {
		artwork, err := ioutil.ReadFile(picPath)
		if err != nil {
			return err
		}
		mime := http.DetectContentType(artwork[:32])
		picture, err := flacpicture.NewFromImageData(flacpicture.PictureTypeFrontCover, "Front cover", artwork, mime)
		if err == nil {
			pictureMeta := picture.Marshal()
			file.Meta = append(file.Meta, &pictureMeta)
		}

	}

	_ = tag.Add(flacvorbis.FIELD_TITLE, songDetail.Name)
	_ = tag.Add(flacvorbis.FIELD_ARTIST, ParseArtist(songDetail))
	if songDetail.Al.Name != "" {
		_ = tag.Add(flacvorbis.FIELD_ALBUM, songDetail.Al.Name)
	}
	_ = tag.Add(flacvorbis.FIELD_DESCRIPTION, musicMarker)

	tagMeta := tag.Marshal()

	var idx int
	for i, m := range file.Meta {
		if m.Type == flac.VorbisComment {
			idx = i
			break
		}
	}
	if idx > 0 {
		file.Meta[idx] = &tagMeta
	} else {
		file.Meta = append(file.Meta, &tagMeta)
	}

	err = file.Save(musicPath)
	if err != nil {
		return err
	}

	return err
}

// ReadMp3Key 解析 mp3 文件的 163Key
func ReadMp3Key(filePath string) (marker MarkerData, err error) {
	tag, err := id3v2.Open(filePath, id3v2.Options{Parse: true})
	if err != nil {
		return marker, err
	}
	defer func(tag *id3v2.Tag) {
		err := tag.Close()
		if err != nil {
			log.Errorln(err)
		}
	}(tag)

	var comment string
	frames := tag.GetFrames(tag.CommonID("Comments"))
	if len(frames) != 0 {
		val, ok := frames[0].(id3v2.CommentFrame)
		if !ok {
			return marker, fmt.Errorf("File :\"%s\" Couldn't assert comment frame ", filePath)
		}
		comment = val.Text
	}

	if strings.Contains(comment, "163 key(Don't modify):") {
		markerText := strings.Replace(comment, "163 key(Don't modify):", "", 1)
		markerJson := strings.Replace(Decrypt163key(markerText), "music:", "", 1)
		var marker MarkerData
		_ = json.Unmarshal([]byte(markerJson), &marker)
		return marker, err
	}

	return marker, fmt.Errorf("File :\"%s\" Invaid Comment Frame ", filePath)
}

// ReadFlacKey 解析 flac 文件的 163Key
func ReadFlacKey(filePath string) (marker MarkerData, err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return marker, err
	}
	flacFile, err := flac.ParseMetadata(file)
	if err != nil {
		return marker, err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Errorln(err)
		}
	}(file)

	var tag *flacvorbis.MetaDataBlockVorbisComment
	for _, meta := range flacFile.Meta {
		if meta.Type == flac.VorbisComment {
			tag, err = flacvorbis.ParseFromMetaDataBlock(*meta)
			if err != nil {
				panic(err)
			}
		}
	}

	comment, err := tag.Get("DESCRIPTION")
	if err != nil {
		return marker, err
	}
	if strings.Contains(comment[0], "163 key(Don't modify):") && len(comment) != 0 {
		markerText := strings.Replace(comment[0], "163 key(Don't modify):", "", 1)
		markerJson := strings.Replace(Decrypt163key(markerText), "music:", "", 1)
		var marker MarkerData
		_ = json.Unmarshal([]byte(markerJson), &marker)
		return marker, err
	}

	return marker, fmt.Errorf("File :\"%s\" Invaid Comment Frame ", filePath)
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
