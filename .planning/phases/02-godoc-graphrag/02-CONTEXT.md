# Phase 2: 문서화 — GoDoc 예시 확충 및 GraphRAG 실전 예제 추가 - Context

**Gathered:** 2026-03-30
**Status:** Ready for planning

<domain>
## Phase Boundary

GoDoc Example 함수(`graph/example_test.go`) 추가와 README.md에 GraphRAG 실전 예제 섹션 추가.
현재 Example 함수가 0개 — `go doc`으로 확인 가능한 runnable 예시 전무.
README Accuracy 테이블에 NMI 값 미기입, NumRuns API 미문서화 상태.

</domain>

<decisions>
## Implementation Decisions

### GoDoc Example 범위
- 파일: `graph/example_test.go` 신규 생성
- 대상 API: `ExampleNewLouvain`, `ExampleNewLeiden`, `ExampleNodeRegistry` 세 개 핵심
- 복잡도: 실용적 (3-5줄 그래프 생성 + Detect 호출) — runnable, `// Output:` 불필요

### GraphRAG 실전 예제
- 위치: README.md 내 `## GraphRAG Example` 신규 섹션
- 시나리오: 엔티티 추출 결과를 그래프로 구성 → Leiden 실행 → 커뮤니티별 컨텍스트 윈도 구성
- NodeRegistry 활용: 포함 — GraphRAG는 스트링 엔티티명 사용

### README 업데이트
- Accuracy 테이블: Football + Polbooks NMI 값 기입 (accuracy_test.go 로그에서 확인 가능)
- NumRuns 도큐: LeidenOptions API 레퍼런스 영역에 한 줄 소개 추가
- API 레퍼런스 테이블: NumRuns 필드 항목 추가 (전체 재작성 아님)

### Claude's Discretion
- GraphRAG 예제 코드의 엔티티/관계 데이터는 현실적으로 보이는 픽션으로 작성
- Example 함수의 그래프 크기 (3-5노드면 충분)
- README 섹션 순서 (GraphRAG Example은 Quick Start 직후 또는 Performance 전)

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `graph/detector.go`: `NewLouvain(LouvainOptions{})`, `NewLeiden(LeidenOptions{})` — Example 대상
- `graph/registry.go`: `NewNodeRegistry()`, `r.Add()`, `r.ID()`, `r.Label()` — NodeRegistry Example 대상
- `graph/graph.go`: `NewGraph(directed bool)`, `g.AddEdge()` — 그래프 생성 패턴
- `graph/accuracy_test.go`: Louvain/Leiden NMI 로그 출력 (Football, Polbooks 값 참조 가능)

### Established Patterns
- 패키지명: `package graph_test` (external test package, go doc에 노출)
- import: `"github.com/bluuewhale/loom/graph"`
- 기존 테스트는 모두 `package graph` (internal)

### Integration Points
- `graph/example_test.go` 신규 파일 — 기존 파일 수정 없음
- `README.md` 업데이트 — GraphRAG Example 섹션 + Accuracy 테이블 + NumRuns 한 줄

</code_context>

<specifics>
## Specific Ideas

- GraphRAG 예제: "AI 논문에서 엔티티(연구자, 개념, 데이터셋) 추출 → 공저/인용 관계로 그래프 구성 → Leiden으로 연구 그룹 클러스터링 → 클러스터별 RAG 컨텍스트 윈도 구성" 시나리오
- Accuracy 테이블 NMI 값: `go test ./graph/ -v -run "TestLouvain.*NMI|TestLeiden.*NMI"` 로그 참조
- NumRuns 추가 위치: README `### LeidenOptions` 코드 블록에 필드 추가

</specifics>

<deferred>
## Deferred Ideas

- examples/ 별도 디렉토리 (실행 가능한 main.go) — 범위 외
- 전체 API 레퍼런스 테이블 재작성 — 범위 외
- NumRuns 전용 별도 섹션 — 범위 외

</deferred>
