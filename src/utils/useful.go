package utils

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/mholt/archiver/v4"
)

// Prints out a warning message to the user to not stop the program while it is downloading
func PrintWarningMsg() {
	color.Yellow("CAUTION:")
	color.Yellow("Please do NOT terminate the program while it is downloading unless you really have to!")
	color.Yellow("Doing so MAY result in incomplete downloads and corrupted files.")
	fmt.Println()
}

// Returns the cookie info for the specified site
//
// Will panic if the site does not match any of the cases
func GetSessionCookieInfo(site string) *cookieInfo {
	switch site {
	case FANTIA:
		return &cookieInfo{
			Domain:   "fantia.jp",
			Name:     "_session_id",
			SameSite: http.SameSiteLaxMode,
		}
	case PIXIV_FANBOX:
		return &cookieInfo{
			Domain:   ".fanbox.cc",
			Name:     "FANBOXSESSID",
			SameSite: http.SameSiteNoneMode,
		}
	case PIXIV:
		return &cookieInfo{
			Domain:   ".pixiv.net",
			Name:     "PHPSESSID",
			SameSite: http.SameSiteNoneMode,
		}
	case KEMONO:
		return &cookieInfo{
			Domain: "kemono.party",
			Name:   "session",
			SameSite: http.SameSiteNoneMode,
		}
	default:
		panic(
			fmt.Errorf(
				"error %d, invalid site, \"%s\" in GetSessionCookieInfo",
				DEV_ERROR,
				site,
			),
		)
	}
}

// Returns a readable format of the website name for the user
//
// Will panic if the site string doesn't match one of its cases.
func GetReadableSiteStr(site string) string {
	switch site {
	case FANTIA:
		return FANTIA_TITLE
	case PIXIV_FANBOX:
		return PIXIV_FANBOX_TITLE
	case PIXIV:
		return PIXIV_TITLE
	case KEMONO:
		return KEMONO_TITLE
	default:
		// panic since this is a dev error
		panic(
			fmt.Errorf(
				"error %d: invalid website, \"%s\", in GetReadableSiteStr",
				DEV_ERROR,
				site,
			),
		)
	}
}

// Uses bufio.Reader to read a line from a file and returns it as a byte slice
//
// Mostly thanks to https://devmarkpro.com/working-big-files-golang
func ReadLine(reader *bufio.Reader) ([]byte, error) {
	var err error
	var isPrefix = true
	var totalLine, line []byte

	// Read until isPrefix is false as
	// that means the line has been fully read
	for isPrefix && err == nil {
		line, isPrefix, err = reader.ReadLine()
		totalLine = append(totalLine, line...)
	}
	return totalLine, err
}

// For the exported cookies in JSON instead of Netscape format
type ExportedCookies []struct {
	Domain   string  `json:"domain"`
	Expire   float64 `json:"expirationDate"`
	HttpOnly bool    `json:"httpOnly"`
	Name     string  `json:"name"`
	Path     string  `json:"path"`
	Secure   bool    `json:"secure"`
	Value    string  `json:"value"`
	Session  bool    `json:"session"`
}

// parse the Netscape cookie file generated by extensions like Get cookies.txt LOCALLY
func ParseNetscapeCookieFile(filePath, sessionId, website string) ([]*http.Cookie, error) {
	if filePath != "" && sessionId != "" {
		return nil, fmt.Errorf(
			"error %d: cannot use both cookie file and session id flags",
			INPUT_ERROR,
		)
	}

	sessionCookieInfo := GetSessionCookieInfo(website)
	sessionCookieName := sessionCookieInfo.Name
	sessionCookieSameSite := sessionCookieInfo.SameSite

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf(
			"error %d: opening cookie file at %s, more info => %v",
			OS_ERROR,
			filePath,
			err,
		)
	}
	defer f.Close()

	var cookies []*http.Cookie
	if ext := filepath.Ext(filePath); ext == ".txt" {
		reader := bufio.NewReader(f)
		for {
			lineBytes, err := ReadLine(reader)
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, fmt.Errorf(
					"error %d: reading cookie file at %s, more info => %v",
					OS_ERROR,
					filePath,
					err,
				)
			}

			line := strings.TrimSpace(string(lineBytes))

			// skip empty lines and comments
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			// split the line
			cookieInfos := strings.Split(line, "\t")
			if len(cookieInfos) < 7 {
				// too few values will be ignored
				continue
			}

			cookieName := cookieInfos[5]
			if cookieName != sessionCookieName {
				// not the session cookie
				continue
			}

			// parse the values
			cookie := http.Cookie{
				Name:     cookieName,
				Value:    cookieInfos[6],
				Domain:   cookieInfos[0],
				Path:     cookieInfos[2],
				Secure:   cookieInfos[3] == "TRUE",
				HttpOnly: true,
				SameSite: sessionCookieSameSite,
			}

			expiresUnixStr := cookieInfos[4]
			if expiresUnixStr != "" {
				expiresUnixInt, err := strconv.Atoi(expiresUnixStr)
				if err != nil {
					// should never happen but just in case
					errMsg := fmt.Sprintf(
						"error %d: parsing cookie expiration time, \"%s\", more info => %v",
						UNEXPECTED_ERROR,
						expiresUnixStr,
						err,
					)
					color.Red(errMsg)
					continue
				}
				if expiresUnixInt > 0 {
					cookie.Expires = time.Unix(int64(expiresUnixInt), 0)
				}
			}
			cookies = append(cookies, &cookie)
		}
	} else if ext == ".json" {
		var exportedCookies ExportedCookies
		if err := json.NewDecoder(f).Decode(&exportedCookies); err != nil {
			return nil, fmt.Errorf(
				"error %d: failed to decode cookie JSON file at %s, more info => %v",
				JSON_ERROR,
				filePath,
				err,
			)
		}

		for _, cookie := range exportedCookies {
			if cookie.Name != sessionCookieName {
				// not the session cookie
				continue
			}

			parsedCookie := &http.Cookie{
				Name:     cookie.Name,
				Value:    cookie.Value,
				Domain:   cookie.Domain,
				Path:     cookie.Path,
				Secure:   cookie.Secure,
				HttpOnly: cookie.HttpOnly,
				SameSite: sessionCookieSameSite,
			}
			if !cookie.Session {
				parsedCookie.Expires = time.Unix(int64(cookie.Expire), 0)
			}

			cookies = append(cookies, parsedCookie)
		}
	} else {
		return nil, fmt.Errorf(
			"error %d: invalid cookie file extension, \"%s\", at %s...\nOnly .txt and .json files are supported",
			INPUT_ERROR,
			ext,
			filePath,
		)
	}

	if len(cookies) == 0 {
		return nil, fmt.Errorf(
			"error %d: no session cookie found in cookie file at %s for website %s",
			INPUT_ERROR,
			filePath,
			GetReadableSiteStr(website),
		)
	}
	return cookies, nil
}

// check page nums if they are in the correct format.
//
// E.g. "1-10" is valid, but "0-9" is not valid because "0" is not accepted
// If the page nums are not in the correct format, os.Exit(1) is called
func ValidatePageNumInput(baseSliceLen int, pageNums []string, errMsgs []string) {
	pageNumsLen := len(pageNums)
	if baseSliceLen != pageNumsLen {
		if len(errMsgs) > 0 {
			for _, errMsg := range errMsgs {
				color.Red(errMsg)
			}
		} else {
			color.Red("Error: %d URLs provided, but %d page numbers provided.", baseSliceLen, pageNumsLen)
			color.Red("Please provide the same number of page numbers as the number of URLs.")
		}
		os.Exit(1)
	}

	valid, outlier := SliceMatchesRegex(PAGE_NUM_REGEX, pageNums)
	if !valid {
		color.Red("Invalid page number format: %s", outlier)
		color.Red("Please follow the format, \"1-10\", as an example.")
		color.Red("Note that \"0\" are not accepted! E.g. \"0-9\" is invalid.")
		os.Exit(1)
	}
}

// Returns the min, max, hasMaxNum, and error from the given string of "num" or "min-max"
//
// E.g.
//
//		"1-10" => 1, 10, true, nil
//		"1" => 1, 1, true, nil
//	 "" => 1, 1, false, nil (defaults to min = 1, max = inf)
func GetMinMaxFromStr(numStr string) (int, int, bool, error) {
	if numStr == "" {
		// defaults to min = 1, max = inf
		return 1, 1, false, nil
	}

	var err error
	var min, max int
	if strings.Contains(numStr, "-") {
		nums := strings.SplitN(numStr, "-", 2)
		min, err = strconv.Atoi(nums[0])
		if err != nil {
			return -1, -1, false, fmt.Errorf(
				"error %d: failed to convert min page number, \"%s\", to int",
				UNEXPECTED_ERROR,
				nums[0],
			)
		}

		max, err = strconv.Atoi(nums[1])
		if err != nil {
			return -1, -1, false, fmt.Errorf(
				"error %d: failed to convert max page number, \"%s\", to int",
				UNEXPECTED_ERROR,
				nums[1],
			)
		}

		if min > max {
			min, max = max, min
		}
	} else {
		min, err = strconv.Atoi(numStr)
		if err != nil {
			return -1, -1, false, fmt.Errorf(
				"error %d: failed to convert page number, \"%s\", to int",
				UNEXPECTED_ERROR,
				numStr,
			)
		}
		max = min
	}
	return min, max, true, nil
}

// Returns a random time.Duration between the given min and max arguments
func GetRandomTime(min, max float64) time.Duration {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomDelay := min + r.Float64()*(max-min)
	return time.Duration(randomDelay*1000) * time.Millisecond
}

// Returns a random time.Duration between the defined min and max delay values in the contants.go file
func GetRandomDelay() time.Duration {
	return GetRandomTime(MIN_RETRY_DELAY, MAX_RETRY_DELAY)
}

// Checks if the given str is in the given arr and returns a boolean
func SliceContains(arr []string, str string) bool {
	for _, el := range arr {
		if el == str {
			return true
		}
	}
	return false
}

type SliceTypes interface {
	~string | ~*string
}

// Removes duplicates from the given slice.
func RemoveSliceDuplicates[T SliceTypes](s []T) []T {
	var result []T
	seen := make(map[T]struct{})
	for _, v := range s {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

// Used for removing duplicate IDs with its corresponding page number from the given slices.
//
// Returns the the new idSlice and pageSlice with the duplicates removed.
func RemoveDuplicateIdAndPageNum[T SliceTypes](idSlice, pageSlice []T) ([]T, []T) {
	var idResult, pageResult []T
	seen := make(map[T]struct{})
	for idx, v := range idSlice {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			idResult = append(idResult, v)
			pageResult = append(pageResult, pageSlice[idx])
		}
	}
	return idResult, pageResult
}

// Checks if the slice of string contains the target str
//
// Otherwise, os.Exit(1) is called after printing error messages for the user to read
func ValidateStrArgs(str string, slice, errMsgs []string) string {
	if SliceContains(slice, str) {
		return str
	}

	if len(errMsgs) > 0 {
		for _, msg := range errMsgs {
			color.Red(msg)
		}
	} else {
		color.Red(
			fmt.Sprintf("Input error, got: %s", str),
		)
	}
	color.Red(
		fmt.Sprintf(
			"Expecting one of the following: %s",
			strings.TrimSpace(strings.Join(slice, ", ")),
		),
	)
	os.Exit(1)
	return ""
}

// Validates if the slice of strings contains only numbers
// Otherwise, os.Exit(1) is called after printing error messages for the user to read
func ValidateIds(args []string) {
	for _, id := range args {
		if !NUMBER_REGEX.MatchString(id) {
			color.Red("Invalid ID: %s", id)
			color.Red("IDs must be numbers!")
			os.Exit(1)
		}
	}
}

// Same as strings.Join([]string, "\n")
func CombineStringsWithNewline(strs []string) string {
	return strings.Join(strs, "\n")
}

// Extract all files from the given archive file to the given destination
//
// Code based on https://stackoverflow.com/a/24792688/2737403
func ExtractFiles(src, dest string, ignoreIfMissing bool) error {
	if !PathExists(src) {
		if ignoreIfMissing {
			return nil
		} else {
			return fmt.Errorf(
				"error %d: %s does not exist",
				OS_ERROR,
				src,
			)
		}
	}

	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf(
			"error %d: unable to open zip file %s",
			OS_ERROR,
			src,
		)
	}
	defer f.Close()

	format, archiveReader, err := archiver.Identify(
		filepath.Base(src),
		f,
	)
	if err == archiver.ErrNoMatch {
		return fmt.Errorf(
			"error %d: %s is not a valid zip file",
			OS_ERROR,
			src,
		)
	} else if err != nil {
		return err
	}

	var input io.Reader
	if decom, ok := format.(archiver.Decompressor); ok {
		rc, err := decom.OpenReader(archiveReader)
		if err != nil {
			return err
		}
		input = rc
		defer rc.Close()
	} else {
		input = archiveReader
	}

	if ex, ok := format.(archiver.Extractor); ok {
		handler := func(ctx context.Context, file archiver.File) error {
			extractedFilePath := filepath.Join(dest, file.NameInArchive)
			os.MkdirAll(filepath.Dir(extractedFilePath), 0666)

			af, err := file.Open()
			if err != nil {
				return err
			}
			defer af.Close()

			out, err := os.OpenFile(
				extractedFilePath,
				os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
				file.Mode(),
			)
			if err != nil {
				return err
			}
			defer out.Close()

			_, err = io.Copy(out, af)
			if err != nil {
				return err
			}
			return nil
		}

		err = ex.Extract(context.Background(), input, nil, handler)
		if err != nil {
			err = fmt.Errorf(
				"error %d: unable to extract zip file %s, more info => %v",
				OS_ERROR,
				src,
				err,
			)
			return err
		}
		return nil
	}

	return fmt.Errorf(
		"error %d: unable to extract zip file %s, more info => %v",
		UNEXPECTED_ERROR,
		src,
		err,
	)
}

// Checks if the slice of string all matches the given regex pattern
//
// Returns true if all matches, false otherwise with the outlier string
func SliceMatchesRegex(regex *regexp.Regexp, slice []string) (bool, string) {
	for _, str := range slice {
		if !regex.MatchString(str) {
			return false, str
		}
	}
	return true, ""
}

// Returns the last part of the given URL string
func GetLastPartOfUrl(url string) string {
	removedParams := strings.SplitN(url, "?", 2)
	splittedUrl := strings.Split(removedParams[0], "/")
	return splittedUrl[len(splittedUrl)-1]
}

// Returns the path without the file extension
func RemoveExtFromFilename(filename string) string {
	return strings.TrimSuffix(filename, filepath.Ext(filename))
}

// Converts a map of string back to a string
func ParamsToString(params map[string]string) string {
	paramsStr := ""
	for key, value := range params {
		paramsStr += fmt.Sprintf("%s=%s&", key, url.QueryEscape(value))
	}
	return paramsStr[:len(paramsStr)-1] // remove the last &
}

// Reads and returns the response body in bytes and closes it
func ReadResBody(res *http.Response) ([]byte, error) {
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		err = fmt.Errorf(
			"error %d: failed to read response body from %s due to %v",
			RESPONSE_ERROR,
			res.Request.URL.String(),
			err,
		)
		return nil, err
	}
	return body, nil
}

// Read the response body and unmarshal it into a interface and returns it
func LoadJsonFromResponse(res *http.Response, format any) error {
	body, err := ReadResBody(res)
	if err != nil {
		return err
	}

	// write to file if debug mode is on
	if DEBUG {
		var prettyJson bytes.Buffer
		err := json.Indent(&prettyJson, body, "", "    ")
		if err != nil {
			color.Red(
				fmt.Sprintf(
					"error %d: failed to indent JSON response body due to %v",
					JSON_ERROR,
					err,
				),
			)
		} else {
			filename := fmt.Sprintf("saved_%s.json", time.Now().Format("2006-01-02_15-04-05"))
			filePath := filepath.Join("json", filename)
			os.MkdirAll(filePath, 0666)
			err = os.WriteFile(filePath, prettyJson.Bytes(), 0666)
			if err != nil {
				color.Red(
					fmt.Sprintf(
						"error %d: failed to write JSON response body to file due to %v",
						UNEXPECTED_ERROR,
						err,
					),
				)
			}
		}
	}

	if err = json.Unmarshal(body, &format); err != nil {
		err = fmt.Errorf(
			"error %d: failed to unmarshal json response from %s due to %v\nBody: %s",
			RESPONSE_ERROR,
			res.Request.URL.String(),
			err,
			string(body),
		)
		return err
	}
	return nil
}

// Detects if the given string contains any passwords
func DetectPasswordInText(text string) bool {
	for _, passwordText := range PASSWORD_TEXTS {
		if strings.Contains(text, passwordText) {
			return true
		}
	}
	return false
}

// Detects if the given string contains any GDrive links and logs it if detected
func DetectGDriveLinks(text, postFolderPath string, isUrl bool) bool {
	gdriveFilename := "detected_gdrive_links.txt"
	gdriveFilepath := filepath.Join(postFolderPath, gdriveFilename)
	driveSubstr := "https://drive.google.com"
	containsGDriveLink := false
	if isUrl && strings.HasPrefix(text, driveSubstr) {
		containsGDriveLink = true
	} else if strings.Contains(text, driveSubstr) {
		containsGDriveLink = true
	}

	if !containsGDriveLink {
		return false
	}

	gdriveText := fmt.Sprintf(
		"Google Drive link detected: %s\n\n",
		text,
	)
	LogMessageToPath(gdriveText, gdriveFilepath)
	return true
}

// Detects if the given string contains any other external file hosting providers links and logs it if detected
func DetectOtherExtDLLink(text, postFolderPath string) bool {
	otherExtFilename := "detected_external_links.txt"
	otherExtFilepath := filepath.Join(postFolderPath, otherExtFilename)
	for _, extDownloadProvider := range EXTERNAL_DOWNLOAD_PLATFORMS {
		if strings.Contains(text, extDownloadProvider) {
			otherExtText := fmt.Sprintf(
				"Detected a link that points to an external file hosting in post's description:\n%s\n\n",
				text,
			)
			LogMessageToPath(otherExtText, otherExtFilepath)
			return true
		}
	}
	return false
}
