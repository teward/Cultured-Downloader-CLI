package utils

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"bufio"
	"strconv"

	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
)

// Prints out a warning message to the user to not stop the program while it is downloading
func PrintWarningMsg() {
	color.Yellow("CAUTION:")
	color.Yellow("Please do NOT stop the program while it is downloading.")
	color.Yellow("Doing so may result in incomplete downloads and corrupted files.")
	fmt.Println()
}

// parse the Netscape cookie file generated by extensions like Get cookies.txt
func ParseNetscapeCookieFile(filePath, sessionId, website string) ([]http.Cookie, error) {
	if filePath != "" && sessionId != "" {
		return nil, fmt.Errorf(
			"error %d: cannot use both cookie file and session id flags",
			INPUT_ERROR,
		)
	}

	var sessionCookieName string
	switch website {
	case FANTIA:
		sessionCookieName = "_session_id"
	case PIXIV_FANBOX:
		sessionCookieName = "FANBOXSESSID"
	case PIXIV:
		sessionCookieName = "PHPSESSID"
	default:
		return nil, fmt.Errorf(
			"error %d: invalid website, \"%s\", in ParseNetscapeCookieFile",
			DEV_ERROR,
			website,
		)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf(
			"error %d: opening cookie file at %s, more info => %v", 
			OS_ERROR, 
			filePath,
			err,
		)
	}

	var cookies []http.Cookie
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

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
			Name: cookieName,
			Value: cookieInfos[6],
			Domain: cookieInfos[0],
			Path: cookieInfos[2],
			Secure: cookieInfos[3] == "TRUE",
			HttpOnly: true,
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
		fmt.Println(cookie)
		cookies = append(cookies, cookie)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf(
			"error %d: reading cookie file at %s, more info => %v",
			OS_ERROR,
			filePath,
			err,
		)
	}

	if len(cookies) == 0 {
		return nil, fmt.Errorf(
			"error %d: no session cookie found in cookie file at %s for website %s",
			INPUT_ERROR,
			filePath,
			API_TITLE_MAP[website],
		)
	}
	return cookies, nil
}

var pageNumsRegex = regexp.MustCompile(`^[1-9]\d*(-[1-9]\d*)?$`)
// check page nums if they are in the correct format.
//
// E.g. "1-10" is valid, but "0-9" is not valid because "0" is not accepted
// If the page nums are not in the correct format, os.Exit(1) is called
func ValidatePageNumInput(baseSliceLen int, pageNums, errMsgs []string) {
	if baseSliceLen != len(pageNums) {
		if len(errMsgs) > 0 {
			for _, errMsg := range errMsgs {
				color.Red(errMsg)
			}
		} else {
			color.Red("Error: %d URLs provided, but %d page numbers provided.", baseSliceLen, len(pageNums))
			color.Red("Please provide the same number of page numbers as the number of URLs.")
		}
		os.Exit(1)
	}

	for _, pageNum := range pageNums {
		if !pageNumsRegex.MatchString(pageNum) {
			color.Red("Invalid page number format: %s", pageNum)
			color.Red("Please follow the format, \"1-10\", as an example.")
			color.Red("Note that \"0\" are not accepted! E.g. \"0-9\" is invalid.")
			os.Exit(1)
		}
	}
}

// Returns a function that will print out a success message in green with a checkmark
//
// To be used with progressbar.OptionOnCompletion (github.com/schollz/progressbar/v3)
func GetCompletionFunc(completionMsg string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, color.GreenString("\033[2K\r✓ %s\n"), completionMsg)
	}
}

// Returns the ProgressBar structure for printing a progress bar for the user to see
func GetProgressBar(total int, desc string, completionFunc func()) *progressbar.ProgressBar {
	return progressbar.NewOptions64(
		int64(total),
		progressbar.OptionUseANSICodes(true),
		progressbar.OptionSetDescription(desc),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionOnCompletion(completionFunc),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionSetWidth(30),
		progressbar.OptionSetRenderBlankState(true),
	)
}

// Returns a random time.Duration between the given min and max arguments
func GetRandomTime(min, max float64) time.Duration {
	rand.Seed(time.Now().UnixNano())
	randomDelay := min + rand.Float64()*(max-min)
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

// Checks if the slice of string contains the target str
//
// Otherwise, os.Exit(1) is called after printing error messages for the user to read
func CheckStrArg(str string, arr, errMsg []string) string {
	str = strings.ToLower(str)
	if SliceContains(arr, str) {
		return str
	}

	if len(errMsg) > 0 {
		for _, msg := range errMsg {
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
			strings.TrimSpace(strings.Join(arr, ", ")),
		),
	)
	os.Exit(1)
	return ""
}

// Checks if the slice of string contains the target str
//
// Otherwise, os.Exit(1) is called after printing error messages for the user to read
func CheckStrArgs(arr []string, str, argName string) *string {
	str = strings.ToLower(str)
	if SliceContains(arr, str) {
		return &str
	} else {
		color.Red("Invalid %s: %s", argName, str)
		color.Red(
			fmt.Sprintf(
				"Valid %ss: %s",
				argName,
				strings.TrimSpace(strings.Join(arr, ", ")),
			),
		)
		os.Exit(1)
		return nil
	}
}

// Since go's flags package doesn't have a way to pass in multiple values for a flag,
// a workaround is to pass in the args like "arg1 arg2 arg3" and
// then split them into a slice by the space delimiter
//
// This function will split based on the given delimiter and return the slice of strings
func SplitArgsWithSep(args, sep string) []string {
	if args == "" {
		return []string{}
	}

	splittedArgs := strings.Split(args, sep)
	seen := make(map[string]bool)
	arr := []string{}
	for _, el := range splittedArgs {
		el = strings.TrimSpace(el)
		if _, value := seen[el]; !value {
			seen[el] = true
			arr = append(arr, el)
		}
	}
	return arr
}

// The same as SplitArgsWithSep, but with the default space delimiter
func SplitArgs(args string) []string {
	return SplitArgsWithSep(args, " ")
}

// Split the given string into a slice of strings with space as the delimiter
// and checks if the slice of strings contains only numbers
//
// Otherwise, os.Exit(1) is called after printing error messages for the user to read
func SplitAndCheckIds(args string) []string {
	ids := SplitArgs(args)
	for _, id := range ids {
		if !NUMBER_REGEX.MatchString(id) {
			color.Red("Invalid ID: %s", id)
			color.Red("IDs must be numbers!")
			os.Exit(1)
		}
	}
	return ids
}

// Same as strings.Join([]string, "\n")
func CombineStringsWithNewline(strs []string) string {
	return strings.Join(strs, "\n")
}

// Unzips the given zip file to the given destination
//
// Code based on https://stackoverflow.com/a/24792688/2737403
func UnzipFile(src, dest string, ignoreIfMissing bool) error {
	if !PathExists(src) {
		if ignoreIfMissing {
			return nil
		} else {
			return errors.New("source zip file does not exist")
		}
	}

	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	os.MkdirAll(dest, 0755)
	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		path := filepath.Join(dest, f.Name)
		// Check for ZipSlip (Directory traversal)
		if !strings.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", path)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
		} else {
			os.MkdirAll(filepath.Dir(path), f.Mode())
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer f.Close()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}
	return nil
}

// Returns the last part of the given URL string
func GetLastPartOfURL(url string) string {
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
func LoadJsonFromResponse(res *http.Response) (interface{}, []byte, error) {
	body, err := ReadResBody(res)
	if err != nil {
		return nil, nil, err
	}

	var post interface{}
	if err = json.Unmarshal(body, &post); err != nil {
		err = fmt.Errorf(
			"error %d: failed to unmarshal json response from %s due to %v",
			RESPONSE_ERROR,
			res.Request.URL.String(),
			err,
		)
		return nil, nil, err
	}
	return post, body, nil
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
