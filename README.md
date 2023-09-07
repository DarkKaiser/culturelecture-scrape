# culturelecture-scrape

<p>
  <img src="https://img.shields.io/badge/Go-00ADD8?style=flat&logo=Go&logoColor=white" />
  <a href="https://github.com/DarkKaiser/culturelecture-scrape/blob/master/LICENSE">
    <img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-yellow.svg" target="_blank" />
  </a>
</p>

이마트/홈플러스/롯데마트 문화센터에서 수강 가능한 모든 강좌를 수집한 후 필터링하여, 그 결과 데이터를 CSV 파일로 저장합니다.

## 설명

### 수집 가능한 이마트

* 여수점
* 순천점

### 수집 가능한 홈플러스

* 광양점
* 순천풍덕점
* 순천점

### 수집 가능한 롯데마트

* 여수점

## Run

`main.go` 소스 파일의 검색년도 및 시즌을 수정한 후 실행하여 문화센터 강좌를 수집합니다.

```go
// 검색년도
var searchYear = "2023"

// 검색시즌(봄, 여름, 가을, 겨울)
var searchSeason = "가을"
```

## Scraped Files

`culturelecture-scrape-YYYYMMDDhhmmss.csv` : 수집된 문화센터 강좌<br /><br />
`culturelecture-scrape.xlsx` : 수집된 문화센터 강좌(CSV)를 편하게 보기 위한 엑셀 파일

## 🤝 Contributing

Contributions, issues and feature requests are welcome.<br />
Feel free to check [issues page](https://github.com/DarkKaiser/culturelecture-scrape/issues) if you want to contribute.

## Author

👤 **DarkKaiser**

- Blog: [@DarkKaiser](http://www.darkkaiser.com)
- Github: [@DarkKaiser](https://github.com/DarkKaiser)
