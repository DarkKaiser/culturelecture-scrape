package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
)

const (
	homeplusCultureBaseURL = "http://school.homeplus.co.kr"
)

/*
 * 점포
 */
var homeplusStoreCodeMap = map[string]string{
	"0035": "홈플러스 광양점",
	"1009": "홈플러스 순천풍덕점",
	"0030": "홈플러스 순천점",
}

/*
 * 강좌군
 */
var homeplusGroupCodeMap = map[string]string{
	"IF": "유아",
	"BB": "영아",
}

/*
 * 강좌 검색 POST 데이터
 */
type homeplusLectureSearchPostData struct {
	Param [23]string `json:"paramData"`
}

/*
 * Convert JSON to Go struct : https://mholt.github.io/json-to-go/
 */
type homeplusLectureSearchResults struct {
	LectureData []lectureData `json:"d"`
}

type lectureData struct {
	Type                  string `json:"__type"`
	ROWNUMBER             string `json:"ROWNUMBER"`
	MAXCNT                string `json:"MAX_CNT"`
	LectureName           string `json:"LectureName"`
	LectureSubType        string `json:"LectureSubType"`
	LectureType           string `json:"LectureType"`
	LectureBaseID         string `json:"LectureBaseID"`
	LectureTargetName     string `json:"LectureTargetName"`
	LectureTargetNameCode string `json:"LectureTargetNameCode"`
	LectureGroupName      string `json:"LectureGroupName"`
	LectureGroupNameCode  string `json:"LectureGroupNameCode"`
	SubLectureName1       string `json:"SubLectureName1"`
	SubLectureName2       string `json:"SubLectureName2"`
	AgeLectureFr          string `json:"AgeLectureFr"`
	AgeLectureTo          string `json:"AgeLectureTo"`
	DateLectureFrTo       string `json:"DateLectureFrTo"`
	StoreName             string `json:"StoreName"`
	StoreCode             string `json:"StoreCode"`
	TeacherName           string `json:"TeacherName"`
	TeacherCode           string `json:"TeacherCode"`
	TuitionFee            string `json:"TuitionFee"`
	TuitionFeeDC          string `json:"TuitionFeeDC"`
	IsShowDcFee           string `json:"IsShowDcFee"`
	MaterialCost          string `json:"MaterialCost"`
	TextBook              string `json:"TextBook"`
	LectureRoomName       string `json:"LectureRoomName"`
	MinMember             string `json:"MinMember"`
	LimitCancel           string `json:"LimitCancel"`
	LectureInfo           string `json:"LectureInfo"`
	LectureDesc           string `json:"LectureDesc"`
	YYYY                  string `json:"YYYY"`
	Season                string `json:"Season"`
	LectureMasterID       string `json:"LectureMasterID"`
	IsOnlyLecture         string `json:"IsOnlyLecture"`
	DCValue               string `json:"DCValue"`
	AdmitLimitType        string `json:"AdmitLimitType"`
	AdmitLimit            string `json:"AdmitLimit"`
	RegStatus             string `json:"RegStatus"`
	DisplayToWeb          string `json:"DisplayToWeb"`
	LectureTime           string `json:"LectureTime"`
	LectureDay            string `json:"LectureDay"`
	LectureCount          string `json:"LectureCount"`
	ClassCount            string `json:"ClassCount"`
	IconSrc               string `json:"IconSrc"`
	LectureStatus         string `json:"LectureStatus"`
	ImgSrc                string `json:"ImgSrc"`
	AdmitValid            string `json:"AdmitValid"`
	DeadLine              string `json:"DeadLine"`
}

func scrapeHomeplusCultureLecture(mainC chan<- []cultureLecture) {
	log.Println("홈플러스 문화센터 강좌 수집을 시작합니다.")

	c := make(chan cultureLecture, 10)

	// 각 점포 및 강좌군의 검색까지 병렬화(goroutine)하면, 검색 결과의 데이터 갯수가 매번 다르게 반환되므로 병렬화를 하지 않음!!!
	// 문제에 대한 원인은 알 수 없음
	count := 0
	for storeCode, storeName := range homeplusStoreCodeMap {
		for groupCode, _ := range homeplusGroupCodeMap {
			clPageURL := homeplusCultureBaseURL + "/Lecture/SearchLectureInfo.aspx/LectureSearchResult"

			m := 1
			n := 0
			for {
				lspd := generateHomeplusLectureSearchPostData(storeCode, groupCode, m, n)
				lspdJSONBytes, err := json.Marshal(lspd)
				checkErr(err)

				reqBody := bytes.NewBuffer(lspdJSONBytes)
				res, err := http.Post(clPageURL, "application/json; charset=utf-8", reqBody)
				checkErr(err)
				checkStatusCode(res)

				defer res.Body.Close()

				resBodyBytes, err := ioutil.ReadAll(res.Body)
				checkErr(err)

				var lectureSearchResult homeplusLectureSearchResults
				err = json.Unmarshal(resBodyBytes, &lectureSearchResult)
				checkErr(err)

				if len(lectureSearchResult.LectureData) == 0 {
					break
				}

				for i := range lectureSearchResult.LectureData {
					count += 1
					go extractHomeplusCultureLecture(clPageURL, storeName, lectureSearchResult.LectureData[i], c)
				}

				m += 1
				n = m
			}
		}
	}

	var cultureLectures []cultureLecture
	for i := 0; i < count; i++ {
		cultureLecture := <-c
		if len(cultureLecture.title) > 0 {
			cultureLectures = append(cultureLectures, cultureLecture)
		}
	}

	log.Println("홈플러스 문화센터 강좌 수집이 완료되었습니다. 총 " + strconv.Itoa(len(cultureLectures)) + "개의 강좌가 수집되었습니다.")

	mainC <- cultureLectures
}

func generateHomeplusLectureSearchPostData(storeCode string, groupCode string, m int, n int) *homeplusLectureSearchPostData {
	lspd := homeplusLectureSearchPostData{}

	lspd.Param[0] = "H"                              // H : 홈페이지, M : 모바일
	lspd.Param[1] = storeCode                        //
	lspd.Param[2] = ""                               //
	lspd.Param[3] = ""                               //
	lspd.Param[4] = ""                               //
	lspd.Param[5] = groupCode                        //
	lspd.Param[6] = ""                               //
	lspd.Param[7] = ""                               // 일반강좌
	lspd.Param[8] = ""                               // 1일특강
	lspd.Param[9] = ""                               // 문화행사
	lspd.Param[10] = ""                              //
	lspd.Param[11] = ""                              // 할인여부
	lspd.Param[12] = "1"                             // 전체(''), 접수중('1'), 마감/대기등록('0')
	lspd.Param[13] = "N"                             // 정렬(N:기본값, 1:강좌명순, 2:요일/시간순, 3:수강료순, 4:개강임박순, 5:마감임박순)
	lspd.Param[14] = "http://school.homeplus.co.kr/" //
	lspd.Param[15] = "//imgcdn.homeplus.co.kr/"      //
	lspd.Param[16] = strconv.Itoa(m)                 // 현재 페이지 번호
	lspd.Param[17] = "20"                            // 페이지당 검색할 강좌 갯수 => 검색할 강좌의 갯수가 너무 많은 경우 500 에러가 발생함
	lspd.Param[18] = strconv.Itoa(n)                 // 뒤로 돌아왔을때 기존 페이지 번호
	lspd.Param[19] = "1"                             // 강좌명순(1), 요일*시간순(2), 수강료순(3)
	lspd.Param[20] = "1"                             // 전체(''), 접수중('1'), 마감/대기등록('0')
	lspd.Param[21] = ""                              // 온니강좌
	lspd.Param[22] = ""                              // 테마강좌

	return &lspd
}

func extractHomeplusCultureLecture(clPageURL string, storeName string, ld lectureData, c chan<- cultureLecture) {
	// @@@@@
	//println("###", s.Text())
	c <- cultureLecture{
		storeName: storeName,
		title:     "1",
		teacher:   ld.TeacherName,
		//href:      href,
		//date:      date,
		//time:      time,
		//won:       won,
	}
}
