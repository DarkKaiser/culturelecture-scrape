# culturelecture-scrape

<p>
  <img src="https://img.shields.io/badge/Go-00ADD8?style=flat&logo=Go&logoColor=white" />
  <a href="https://github.com/DarkKaiser/culturelecture-scrape/blob/master/LICENSE">
    <img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-yellow.svg" target="_blank" />
  </a>
</p>

대형마트 문화센터의 강좌 정보를 자동으로 수집하여 CSV 파일로 저장하는 도구입니다.

## 주요 기능

- 이마트/홈플러스/롯데마트 문화센터의 강좌 정보 자동 수집
- 수집된 데이터를 CSV 및 Excel 형식으로 저장
- 연도 및 시즌별 강좌 검색 지원

## 설치 방법

```bash
git clone https://github.com/DarkKaiser/culturelecture-scrape.git
cd culturelecture-scrape
go mod download
```

## 수집 가능한 문화센터 지점

전라남도 지역의 대형마트 문화센터를 지원합니다:

### 이마트
- 여수점
- 순천점

### 홈플러스
- 광양점
- 순천풍덕점
- 순천점

### 롯데마트
- 여수점

## 사용 방법

1. `main.go` 파일에서 검색 조건을 설정합니다:
```go
// 검색년도
var searchYear = "2023"

// 검색시즌(봄, 여름, 가을, 겨울)
var searchSeason = "가을"
```

2. 프로그램을 실행합니다:
```bash
go run main.go
```

## 출력 파일

| 파일명 | 설명 |
|--------|------|
| `culturelecture-scrape-YYYYMMDDhhmmss.csv` | 수집된 강좌 정보 (CSV 형식) |
| `culturelecture-scrape.xlsx` | 수집된 강좌 정보 (Excel 형식) |

## 🤝 Contributing

Contributions, issues and feature requests are welcome.<br />
Feel free to check [issues page](https://github.com/DarkKaiser/culturelecture-scrape/issues) if you want to contribute.

## Author

👤 **DarkKaiser**

- Blog: [@DarkKaiser](http://www.darkkaiser.com)
- Github: [@DarkKaiser](https://github.com/DarkKaiser)
