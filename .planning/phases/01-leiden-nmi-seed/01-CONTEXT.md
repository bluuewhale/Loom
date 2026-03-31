# Phase 1: Leiden NMI 안정성 — seed 의존성 문제 해결 및 알고리즘 수렴 보장 강화 - Context

**Gathered:** 2026-03-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Leiden 알고리즘의 NMI 품질이 seed 값에 따라 크게 달라지는 문제를 해결한다.
현재 테스트는 Seed=2일 때만 NMI>=0.72를 만족하며, Seed=42는 NMI=0.60으로 실패한다.
이 Phase에서는 `LeidenOptions`에 `NumRuns` 옵션을 추가하여 Seed=0(non-deterministic) 모드에서
여러 번 실행한 뒤 최고 Q를 반환하는 multi-run 전략을 구현한다.
Seed!=0인 기존 callers는 단일 run으로 동작을 유지하여 breaking change 없이 호환성을 보장한다.

</domain>

<decisions>
## Implementation Decisions

### 수렴 전략
- Multi-run + best-Q pick: NumRuns=3 (기본값), 각 run은 독립 seed로 실행, 최고 Q 결과 반환
- NumRuns 기본값: 0 = default(3), 1 = single run
- 활성화 조건: Seed=0 일 때만 multi-run (non-deterministic mode). Seed!=0이면 단일 run 유지 (deterministic 보장)
- 추가 수렴 조건 없음 — 기존 bestQ tracking으로 충분

### API Surface (LeidenOptions)
- `LeidenOptions.NumRuns int` 필드 추가 — 0 = default(3), 1 = single run
- 기존 Seed!=0 caller: 동작 변경 없음 (breaking change 없음)
- NumRuns=1: 기존 단일 run 동작 그대로

### 테스트 전략
- 기존 Seed=2 핀 NMI 테스트 유지 + NumRuns=1 명시 — deterministic baseline으로 유지
- 신규 TestLeidenStabilityMultiRun 추가: Seed=0 + NumRuns=3으로 Karate Club NMI threshold 검증
- stability 테스트 범위: Karate Club만 (작은 그래프, 빠른 테스트)

### 멀티런 구현 세부사항
- 멀티런 루프 위치: leidenDetector.Detect 내부 — NumRuns loop이 Detect를 래핑, bestQ 추적
- 각 run의 seed 구성: baseSeed + run index (seed=0이면 time.Now().UnixNano()+int64(i))
- bestQ 선정 시 파티션 복사: 수렴 성공 직후 — 기존 bestSuperPartition deep-copy 패턴 활용

### Claude's Discretion
- NumRuns loop 내부 구조 (runOnce helper 분리 여부 등) — 코드 명료성 기준으로 판단
- 에러 처리: 모든 run이 에러면 마지막 에러 반환

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `leidenDetector.Detect` in `graph/leiden.go` — 멀티런 루프가 들어갈 위치
- `LeidenOptions` struct in `graph/detector.go` — NumRuns 필드 추가 대상
- 기존 bestQ / bestSuperPartition deep-copy 패턴 (leiden.go L76-86) — 재활용 가능
- `newLeidenState`, `acquireLeidenState`, pool 패턴 in `graph/leiden_state.go`

### Established Patterns
- `LouvainOptions.Seed int64` — LeidenOptions도 동일 패턴 따름
- `TestLeidenDeterministic` — 동일 seed 두 번 실행 비교 패턴 (신규 stability 테스트 참고)
- `bestSuperPartition` deep-copy: `make(map[NodeID]int, len(...))` + for-range 복사

### Integration Points
- `graph/detector.go`: `LeidenOptions` struct, `NewLeiden` 함수
- `graph/leiden.go`: `Detect` 메서드 — 멀티런 래퍼 삽입 지점
- `graph/leiden_test.go`: 기존 Seed=2 테스트에 NumRuns=1 추가, 신규 stability 테스트 추가
- `graph/accuracy_test.go`: Leiden NMI 테스트 — Seed=2, NumRuns=1 명시로 업데이트

</code_context>

<specifics>
## Specific Ideas

- Seed!=0 이면 단일 run — 기존 callers(Seed=2, Seed=42 등) 완전 호환
- Seed=0 + NumRuns=N 이면 N번 실행, best Q 반환
- 각 run의 seed: `baseSeed=time.Now().UnixNano()`, run i → `baseSeed + int64(i)`
- 기존 Seed=2 테스트에 `NumRuns: 1` 명시 추가 (의도 명확화)

</specifics>

<deferred>
## Deferred Ideas

- Football / Polbooks에 대한 stability 테스트 — 범위 외, 향후 추가 가능
- Louvain multi-run 지원 — 이 Phase는 Leiden만 대상
- Q 개선 < ε 조기종료 조건 — 현재 bestQ tracking으로 충분

</deferred>
