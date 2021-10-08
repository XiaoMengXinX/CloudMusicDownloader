package downloader

import (
	"encoding/base64"
	"fmt"
	"github.com/XiaoMengXinX/Music163Api-Go/types"
	"github.com/XiaoMengXinX/Music163Api-Go/utils"
)

// ParseArtist 解析歌手数据
func ParseArtist(songDetail types.SongDetailData) string {
	var artists string
	for i, ar := range songDetail.Ar {
		if i == 0 {
			artists = ar.Name
		} else {
			artists = fmt.Sprintf("%s, %s", artists, ar.Name)
		}
	}
	return artists
}

// Decrypt163key 解码 163 key
func Decrypt163key(encrypted string) (decrypted string) {
	data, _ := base64.StdEncoding.DecodeString(encrypted)
	return string(utils.MarkerAesDecryptECB(data))
}
