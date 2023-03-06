package pixivfanbox

import (
	"net/http"

	"github.com/KJHJason/Cultured-Downloader-CLI/api"
	"github.com/KJHJason/Cultured-Downloader-CLI/utils"
)

type PixivFanboxDl struct {
	CreatorIds []string
	PostIds    []string
}

func (pf *PixivFanboxDl) ValidateArgs() {
	utils.ValidateIds(&pf.CreatorIds)
	utils.ValidateIds(&pf.PostIds)
}

type PixivFanboxDlOptions struct {
	DlThumbnails  bool
	DlImages      bool
	DlAttachments bool
	DlGdrive      bool

	SessionCookieId string
	SessionCookies  []http.Cookie
}

func (pf *PixivFanboxDlOptions) ValidateArgs() {
	if pf.SessionCookieId != "" {
		pf.SessionCookies = []http.Cookie{
			api.VerifyAndGetCookie(utils.PIXIV_FANBOX, pf.SessionCookieId),
		}
	}
}
