package downloader

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/XiaoMengXinX/Music163Api-Go/types"
	"github.com/XiaoMengXinX/Music163Api-Go/utils"
	"strconv"
)

func bytesToHexString(src []byte) string {
	res := bytes.Buffer{}
	if src == nil || len(src) <= 0 {
		return ""
	}
	temp := make([]byte, 0)
	for _, v := range src {
		sub := v & 0xFF
		hv := hex.EncodeToString(append(temp, sub))
		if len(hv) < 2 {
			res.WriteString(strconv.FormatInt(int64(0), 10))
		}
		res.WriteString(hv)
	}
	return res.String()
}

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
