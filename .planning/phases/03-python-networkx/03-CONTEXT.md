# Phase 3: 벤치마크 비교 — Python networkx 대비 성능 비교표 작성 (채택 논거) - Context

**Gathered:** 2026-03-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Python networkx(python-louvain) 대비 loom 성능을 실측 비교하여 README에 채택 논거 비교표 추가.
`scripts/compare.py`를 작성하여 실제 측정값을 얻고, README Performance 섹션을 확장.
현재 README는 Go 수치만 있고 경쟁 비교가 없음.

</domain>

<decisions>
## Implementation Decisions

### Python 수치 확보 방법
- Python 뱀치마크 스크립트 실행 — compare.py 작성 후 실제 측정
- 패키지: python-louvain (community 패키지) — 가장 널리 쓰이는 Go 대안
- 버전 표시 포함: Python 3.x + networkx x.x + community x.x

### 비교표 형식
- 위치: README.md Performance 섹션 확장 (별도 파일 아님)
- 지표: Time + Speedup 배율 (e.g. "10x faster")
- 그래프 크기: 1K / 10K 두 시나리오

### 하드웨어 전제 및 교이웉
- 하드웨어 명시: 테이블 footer에 Apple M4 / Python 3.x
- 설치 안내: 표 바로 아래 `pip install networkx python-louvain` 1줄
- compare.py 위치: scripts/compare.py 파일로 커밋

### Claude's Discretion
- compare.py의 그래프 생성 방법 (Erdos-Renyi 또는 Barabasi-Albert)
- 측정 반복 횟수 (timeit 기본값 또는 명시적 N)
- README 비교표 마크다운 레이아웃 세부

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `bench-baseline.txt`: Louvain 10K ~50ms, Leiden 10K ~56ms (Apple M4, arm64)
- `graph/benchmark_test.go`: 기존 Go 벤치마크 패턴 참조
- README.md `## Performance` 섹션: 현재 Go 수치만 있는 테이블 (확장 대상)

### Established Patterns
- 비교 대상: `python-louvain` — `import community; community.best_partition(G)`
- networkx 그래프: `nx.erdos_renyi_graph(n, p)` 또는 `nx.barabasi_albert_graph(n, m)`

### Integration Points
- `scripts/compare.py` 신규 파일
- `README.md` `## Performance` 섹션 확장

</code_context>

<specifics>
## Specific Ideas

- compare.py: `timeit` 모듈로 측정, 결과를 stdout으로 출력 (테이블 형식)
- 1K 그래프: `nx.barabasi_albert_graph(1000, 5)` — 실세계 유사 구조
- 10K 그래프: `nx.barabasi_albert_graph(10000, 5)` — 기존 Go 벤치와 동일 크기
- Go 수치는 bench-baseline.txt 값 사용 (Louvain ~50ms, Leiden ~56ms)

</specifics>

<deferred>
## Deferred Ideas

- 별도 BENCHMARKS.md 파일 — 범위 외
- Memory 비교 — 범위 외 (Time+Speedup만)
- 100K+ 그래프 — 범위 외

</deferred>
