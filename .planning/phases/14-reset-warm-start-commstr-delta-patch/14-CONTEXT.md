# Phase 1: reset() warm-start 최적화 - Context

**Gathered:** 2026-03-31
**Status:** Ready for planning
**Mode:** Auto-generated (infrastructure phase — discuss skipped)

<domain>
## Phase Boundary

`louvainState.reset()` 및 `leidenState.reset()`의 warm-start 경로에서 발생하는 불필요한 전체 노드 순회를 제거한다.

프로파일링 결과:
- `slices.Sort(nodes)`: 610ms — 94K 노드 정렬을 매 Update() 호출마다 수행
- `commStr` 재계산: 940ms — 94K 노드 전체 `g.Strength(n)` 재계산, affected 노드가 2개뿐인 경우에도

최적화 대상:
1. **warm-start 경로에서 sorted node list 캐싱** — 노드 집합이 변하지 않으면 재정렬 불필요
2. **commStr delta 패치** — warm-start 시 affected 커뮤니티의 강도만 갱신 (전체 재계산 대신)

두 최적화 모두 `louvain_state.go`와 `leiden_state.go`에 동일하게 적용. 구조가 동일하므로 한 번에 처리.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure/performance phase.

- sorted node slice 캐싱: `reset()` 내부에서 처리할지, `Graph`에 캐시를 둘지는 구현 시 판단
- commStr delta 패치: affected 노드의 이전 커뮤니티 강도를 빼고 새 강도를 더하는 방식
- backward compatibility: 기존 cold-start 경로 동작 변경 없음
- 기존 테스트 모두 통과해야 함

</decisions>

<code_context>
## Existing Code Insights

### 병목 위치
- `graph/louvain_state.go:49-116` — `louvainState.reset()`
- `graph/leiden_state.go:53-121` — `leidenState.reset()` (동일 구조)

### 핵심 병목 라인
- `louvain_state.go:69`: `slices.Sort(nodes)` — 610ms
- `louvain_state.go:114`: `st.commStr[st.partition[n]] += g.Strength(n)` — 940ms (warm-start step 4)
- `leiden_state.go:74`: `slices.Sort(nodes)` — 동일
- `leiden_state.go:119`: `st.commStr[st.partition[n]] += g.Strength(n)` — 동일

### 관련 벤치마크
- `graph/benchmark_test.go`: `BenchmarkLouvainWarmStart`, `BenchmarkLeidenWarmStart`
- `graph/ego_splitting_test.go`: `BenchmarkEgoSplittingUpdate1Node1Edge`, `TestEgoSplittingUpdateAllocSavings`

### 프로파일링 기반 측정
- Update() 전체: ~175ms/op
- louvainState.reset(): 3.15s / 11.19s Update 총합 = 28%
- 목표: reset() 시간을 warm-start 경로에서 50% 이상 감소

</code_context>

<specifics>
## Specific Ideas

- cold-start 경로는 변경하지 않는다 — 기존 동작 100% 보존
- `louvainState`와 `leidenState` 동시에 수정 — 두 파일 구조가 동일하므로 동일한 패턴 적용
- 기존 warm-start 테스트(`TestLouvainWarmStartSpeedup`, `TestLeidenWarmStartSpeedup`) 통과 유지

</specifics>

<deferred>
## Deferred Ideas

- persona graph Clone() 최적화 (copy-on-write) — 별도 phase로 필요시 추가
- Louvain phase1 map 연산 최적화 — 알고리즘 구조적 한계, 별도 연구 필요

</deferred>
