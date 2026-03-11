# culturelecture-scrape

<p>
  <img src="https://img.shields.io/badge/Go-00ADD8?style=flat&logo=Go&logoColor=white" />
  <a href="https://github.com/DarkKaiser/culturelecture-scrape/blob/master/LICENSE">
    <img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-yellow.svg" target="_blank" />
  </a>
</p>

대형마트 문화센터의 강좌 정보를 자동으로 수집하여 필터링하고 CSV 파일로 저장하는 도구입니다.

## 주요 기능

- **다중 마트 지원**: 이마트, 홈플러스, 롯데마트 문화센터 강좌 동시 수집
- **강력한 필터링**: 수강생 생년월일 기준 나이/개월 수 자동 계산 및 수강 불가 강좌 자동차단
- **편리한 설정**: `culturelecture-scrape.yaml` 환결설정 파일을 통한 검색 조건 중앙 제어
- **표준화된 출력**: 모든 마트의 강좌 데이터를 취합해 단일 CSV(`culturelecture-scrape-YYYY...csv`) 포맷으로 추출

## 수집 대상 지점 (기본 설정)

현재 전라남도 지역 중심으로 기본 설정이 구성되어 있으며, 추후 YAML 파일에서 자유롭게 변경 가능합니다.

- **이마트**: 여수점, 순천점
- **홈플러스**: 광양점, 순천점
- **롯데마트**: 여수점

## 설치 방법

```bash
git clone https://github.com/DarkKaiser/culturelecture-scrape.git
cd culturelecture-scrape
go mod download
```

## 사용 방법

1. 프로젝트 루트 경로의 `culturelecture-scrape.yaml` 파일을 열어 다음 정보를 설정합니다.
   * `search_year`, `search_season`: 수집할 연도 및 학기(봄/여름/가을/겨울)
   * `student`: 수강생 생년월일 (나이 기반 필터링 용도)
   * `providers.emart.auth_token`: 이마트 강좌 조회를 위한 `x-api-key` 지정 (이마트 웹사이트 개발자도구 네트워크 탭 참조)

2. 다음 명령어를 통해 스크래퍼를 실행합니다:
```bash
go run ./cmd/culturelecture-scrape
```

*(선택사항)* 설정 파일을 별도 경로로 지정하여 실행할 수도 있습니다:
```bash
go run ./cmd/culturelecture-scrape -config /path/to/custom-config.yaml
```

## 출력 결과물

| 파일명 | 설명 |
|--------|------|
| `culturelecture-scrape-YYYYMMDD-hhmmss.csv` | 통합 수집 및 필터링 된 최종 강좌 목록 |

## 🤝 Contributing

Contributions, issues and feature requests are welcome.<br />
Feel free to check [issues page](https://github.com/DarkKaiser/culturelecture-scrape/issues) if you want to contribute.

## Author

👤 **DarkKaiser**

- Blog: [@DarkKaiser](http://www.darkkaiser.com)
- Github: [@DarkKaiser](https://github.com/DarkKaiser)
