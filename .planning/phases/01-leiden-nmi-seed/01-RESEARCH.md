# Phase 1: Leiden NMI 안정성 — seed 의존성 문제 해결 및 알고리즘 수렴 보장 강화 - Research

**Researched:** 2026-03-30
**Domain:** Go community detection — Leiden multi-run strategy, LeidenOptions API extension
**Confidence:** HIGH

## Summary

이 Phase는 신규 라이브러리 도입이나 외부 의존성 없이 프로젝트 내부 코드만 수정한다.
`graph/detector.go`의 `LeidenOptions` struct에 `NumRuns int` 필드를 추가하고,
`graph/leiden.go`의 `leidenDetector.Detect` 메서드에 multi-run 래퍼 루프를 삽입하는 것이 전부다.

Seed=0(non-deterministic) 모드에서만 multi-run이 활성화되며, NumRuns=0은 default(3)으로
해석한다. Seed!=0인 기존 caller는 코드 수정 없이 단일 run으로 계속 동작한다.
각 run마다 `baseSeed + int64(i)` seed를 사용하되, baseSeed는 `time.Now().UnixNano()`로
구한다. 기존 `bestQ` / `bestSuperPartition` deep-copy 패턴이 이미 구현되어 있으므로
최고-Q run 선택 로직은 이 패턴을 그대로 재활용한다.

**Primary recommendation:** `leidenDetector.Detect` 내부에 NumRuns 루프를 삽입하고,
각 루프 반복이 기존 단일-run 로직 전체를 실행한 뒤 best Q를 추적하도록 구현한다.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- Multi-run + best-Q pick: NumRuns=3 (기본값), 각 run은 독립 seed로 실행, 최고 Q 결과 반환
- NumRuns 기본값: 0 = default(3), 1 = single run
- 활성화 조건: Seed=0 일 때만 multi-run (non-deterministic mode). Seed!=0이면 단일 run 유지 (deterministic 보장)
- 추가 수렴 조건 없음 — 기존 bestQ tracking으로 충분
- `LeidenOptions.NumRuns int` 필드 추가 — 0 = default(3), 1 = single run
- 기존 Seed!=0 caller: 동작 변경 없음 (breaking change 없음)
- NumRuns=1: 기존 단일 run 동작 그대로
- 기존 Seed=2 핀 NMI 테스트 유지 + NumRuns=1 명시 — deterministic baseline으로 유지
- 신규 TestLeidenStabilityMultiRun 추가: Seed=0 + NumRuns=3으로 Karate Club NMI threshold 검증
- stability 테스트 범위: Karate Club만 (작은 그래프, 빠른 테스트)
- 멀티런 루프 위치: leidenDetector.Detect 내부 — NumRuns loop이 Detect를 래핑, bestQ 추적
- 각 run의 seed 구성: baseSeed + run index (seed=0이면 time.Now().UnixNano()+int64(i))
- bestQ 선정 시 파티션 복사: 수렴 성공 직후 — 기존 bestSuperPartition deep-copy 패턴 활용

### Claude's Discretion
- NumRuns loop 내부 구조 (runOnce helper 분리 여부 등) — 코드 명료성 기준으로 판단
- 에러 처리: 모든 run이 에러면 마지막 에러 반환

### Deferred Ideas (OUT OF SCOPE)
- Football / Polbooks에 대한 stability 테스트 — 범위 외, 향후 추가 가능
- Louvain multi-run 지원 — 이 Phase는 Leiden만 대상
- Q 개선 < ε 조기종료 조건 — 현재 bestQ tracking으로 충분
</user_constraints>

## Standard Stack

이 Phase는 외부 라이브러리를 추가하지 않는다. 기존 표준 라이브러리(`time`, `math/rand`)만 사용.

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `math/rand` | Go stdlib | per-run RNG seeding | 이미 `leiden_state.go`에서 사용 중 |
| `time` | Go stdlib | non-deterministic baseSeed 생성 | 이미 `leiden_state.go` reset()에서 사용 중 |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `time.Now().UnixNano()` per multi-run call | `crypto/rand` | crypto/rand는 오버헤드가 크고 테스트에서 재현성이 없음. 현 패턴이 적합 |

**Installation:** 추가 패키지 없음.

## Architecture Patterns

### Existing Code Structure (변경 대상)

```
graph/
├── detector.go          # LeidenOptions struct — NumRuns 필드 추가
├── leiden.go            # leidenDetector.Detect — multi-run 래퍼 삽입
├── leiden_state.go      # acquireLeidenState, reset() — 재사용 (변경 없음)
├── leiden_test.go       # TestLeidenDeterministic 등 — NumRuns=1 명시 추가
└── accuracy_test.go     # TestLeidenKarateClubAccuracy 등 — NumRuns=1 명시, 신규 stability 테스트 추가
```

### Pattern 1: NumRuns 루프 — Detect 내부 래핑

**What:** `leidenDetector.Detect`의 guard clause 이후, 기존 single-run 로직 전체를
`runOnce` helper(또는 인라인 클로저)로 추출하고, NumRuns 루프에서 반복 호출.
각 호출 결과의 Q를 비교해 최고 Q인 `CommunityResult`를 반환.

**When to use:** Seed=0이고 effectiveNumRuns > 1일 때만 루프 실행.
Seed!=0이거나 effectiveNumRuns==1이면 루프 없이 직접 호출 (기존 동작 완전 보존).

**Seed 결정 로직:**
```go
// Source: 01-CONTEXT.md decisions + leiden_state.go reset() 패턴
effectiveNumRuns := d.opts.NumRuns
if effectiveNumRuns == 0 {
    effectiveNumRuns = 3 // default
}

if d.opts.Seed != 0 || effectiveNumRuns == 1 {
    // 단일 run — 기존 동작 그대로
    return d.runOnce(g, d.opts.Seed)
}

// Seed=0 + multi-run
baseSeed := time.Now().UnixNano()
var bestResult CommunityResult
bestQ := math.Inf(-1)
var lastErr error
for i := 0; i < effectiveNumRuns; i++ {
    runSeed := baseSeed + int64(i)
    res, err := d.runOnce(g, runSeed)
    if err != nil {
        lastErr = err
        continue
    }
    if res.Modularity > bestQ {
        bestQ = res.Modularity
        bestResult = res
    }
}
if bestQ == math.Inf(-1) {
    return CommunityResult{}, lastErr
}
return bestResult, nil
```

### Pattern 2: runOnce helper — 기존 Detect 로직 캡슐화

**What:** 현재 `Detect` 내부의 전체 단일-run 알고리즘을 `runOnce(g *Graph, seed int64) (CommunityResult, error)`로 추출.
`Detect`는 multi-run 결정 로직만 담당, `runOnce`가 실제 Leiden 알고리즘 실행.

**When to use:** runOnce helper 분리 여부는 Claude's Discretion. 인라인 루프 vs helper 모두 유효.
helper 분리 시 코드 가독성이 높아지고 단위 테스트 용이. 코드 명료성 기준으로 판단.

**Example (현재 Detect 시그니처에서 추출):**
```go
// Source: graph/leiden.go 기존 구조 기반
func (d *leidenDetector) runOnce(g *Graph, seed int64) (CommunityResult, error) {
    // 현재 Detect의 guard clause 이후 로직 전체를 이동
    // (resolution 계산, nodeMapping 초기화, main loop, reconstruct 등)
}
```

### Pattern 3: LeidenOptions NumRuns 필드 추가

**What:** `graph/detector.go`의 `LeidenOptions` struct에 `NumRuns int` 필드 추가.
LouvainOptions 패턴을 따라 zero value가 meaningful default로 동작하도록 godoc 작성.

```go
// Source: graph/detector.go LeidenOptions 기존 패턴
type LeidenOptions struct {
    Resolution       float64
    Seed             int64
    MaxIterations    int
    Tolerance        float64
    InitialPartition map[NodeID]int
    // NumRuns specifies how many independent runs to execute when Seed=0
    // (non-deterministic mode). The run with the highest modularity Q is returned.
    // 0 = default (3 runs), 1 = single run (same as Seed!=0 behavior).
    // Ignored when Seed != 0.
    NumRuns int
}
```

### Pattern 4: 기존 테스트에 NumRuns=1 명시

**What:** `graph/leiden_test.go`의 `TestLeidenKarateClubAccuracy`, `TestLeidenDeterministic` 등
Seed=2를 사용하는 모든 기존 Leiden 테스트에 `NumRuns: 1`을 명시.
의도: Seed!=0이면 NumRuns 무시이지만, 테스트 독자가 "이 테스트는 단일 run baseline임"을 명확히 읽을 수 있도록.

```go
// Before:
det := NewLeiden(LeidenOptions{Seed: 2})
// After:
det := NewLeiden(LeidenOptions{Seed: 2, NumRuns: 1})
```

### Pattern 5: 신규 TestLeidenStabilityMultiRun

**What:** `graph/leiden_test.go` 또는 `graph/accuracy_test.go`에 추가.
Seed=0 + NumRuns=3으로 Karate Club을 실행하고 NMI >= 0.72 threshold를 검증.

```go
// Source: 01-CONTEXT.md decisions + TestLeidenKarateClubAccuracy 패턴
func TestLeidenStabilityMultiRun(t *testing.T) {
    g := buildKarateClubLeiden()
    det := NewLeiden(LeidenOptions{Seed: 0, NumRuns: 3})
    res, err := det.Detect(g)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    gt := make(map[NodeID]int, len(testdata.KarateClubPartition))
    for k, v := range testdata.KarateClubPartition {
        gt[NodeID(k)] = v
    }
    score := nmi(res.Partition, gt)
    if score < 0.72 {
        t.Errorf("NMI = %.4f, want >= 0.72 (multi-run stability)", score)
    }
    t.Logf("MultiRun: Q=%.4f communities=%d NMI=%.4f",
        res.Modularity, uniqueCommunities(res.Partition), score)
}
```

### Anti-Patterns to Avoid

- **기존 Seed!=0 동작 변경:** Seed!=0이면 NumRuns를 완전히 무시해야 함. effectiveNumRuns를 계산하기 전에 Seed!=0 체크를 먼저 수행.
- **pool state 누수:** `runOnce` 내에서 `acquireLeidenState` / `releaseLeidenState` 쌍을 완결해야 함. 루프 외부에서 한 번만 acquire하면 안 됨 — 각 run은 독립된 state를 가져야 함.
- **bestResult가 초기화되지 않은 채 반환:** 모든 run이 에러인 경우 반드시 lastErr를 반환. bestQ가 -Inf로 남아 있으면 zero-value CommunityResult 반환하지 않도록.
- **Passes/Moves 집계 혼동:** multi-run에서 반환하는 Passes/Moves는 best-Q run의 값이어야 함. 전체 run의 합산이 아님.
- **time.Now() 호출을 루프 내부에서:** baseSeed는 루프 시작 전 한 번만 계산. 루프 내에서 매번 time.Now()를 호출하면 seed 다양성이 낮아짐 (동일 나노초 가능).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| 최고-Q 파티션 복사 | 직접 새 구현 | 기존 `bestSuperPartition` deep-copy 패턴 (leiden.go L116-121) | 이미 검증된 패턴: `make(map[NodeID]int, len(...))` + for-range 복사 |
| non-deterministic seed | crypto/rand 등 | `time.Now().UnixNano()` (기존 패턴) | leiden_state.go reset()에서 동일 패턴 사용 중. 새 패턴 도입 불필요 |
| NMI 계산 | 직접 구현 | `nmi()` in `testhelpers_test.go` | Phase 04에서 추출된 공유 helper. 재사용 |

## Common Pitfalls

### Pitfall 1: pool state를 run 간에 공유
**What goes wrong:** multi-run 루프 외부에서 `acquireLeidenState`를 한 번만 호출하고
각 run에서 동일 state를 reset()으로 재사용하면, 이전 run의 bestSuperPartition이 다음 run의
state.reset()으로 clear될 때 이미 저장된 bestResult.Partition을 덮어쓸 수 있음.
**Why it happens:** `bestSuperPartition`은 state map을 가리키는 포인터이므로, state.reset() 시 clear되면 bestResult 내 파티션도 무효화됨.
**How to avoid:** 각 `runOnce` 호출이 독립적으로 `acquireLeidenState` / `releaseLeidenState`를 수행. 또는 `runOnce` 내부에서 state를 획득/반환.
**Warning signs:** TestLeidenStabilityMultiRun이 모든 run에서 동일한 Q를 반환하거나 첫 번째 run 결과만 반환.

### Pitfall 2: Seed=0 판별과 NumRuns 적용 순서 혼동
**What goes wrong:** Seed!=0인데 effectiveNumRuns=3으로 루프를 실행하면,
deterministic 테스트(`TestLeidenDeterministic`)가 실패할 수 있음.
**Why it happens:** NumRuns 기본값(0→3) 계산을 Seed 체크보다 먼저 적용.
**How to avoid:** 코드 진입 시 `if d.opts.Seed != 0 { return d.runOnce(g, d.opts.Seed) }` 를 NumRuns 해석보다 먼저 배치.
**Warning signs:** TestLeidenDeterministic 실패 (Q가 두 run 간 달라짐).

### Pitfall 3: baseSeed 계산 위치
**What goes wrong:** 루프 내부에서 매 iteration마다 `time.Now().UnixNano()`를 호출하면
고해상도 시계가 아닌 환경에서 동일 나노초 타임스탬프가 반복되어 seed 다양성 저하.
**Why it happens:** time.Now() 해상도는 OS에 따라 ~100ns~1ms.
**How to avoid:** `baseSeed := time.Now().UnixNano()`를 루프 시작 전 한 번만 호출.
각 run seed = `baseSeed + int64(i)` — 이 패턴은 CONTEXT.md에서 명시적으로 결정됨.
**Warning signs:** 여러 run이 동일 partition을 반환함.

### Pitfall 4: 에러 처리에서 성공한 run 결과 유실
**What goes wrong:** 일부 run이 에러를 반환하는 경우, 성공한 run의 bestResult를 버리고 에러만 반환.
**Why it happens:** 에러가 발생한 즉시 return하는 패턴을 multi-run에 그대로 적용.
**How to avoid:** 모든 run을 실행하고, 성공한 run이 하나라도 있으면 bestResult 반환. 모든 run이 에러인 경우에만 lastErr 반환.
**Warning signs:** 드문 에러가 발생할 때 유효한 결과를 받지 못함.

## Code Examples

### 기존 bestSuperPartition deep-copy 패턴 (재활용)
```go
// Source: graph/leiden.go L116-121
bestSuperPartition = make(map[NodeID]int, len(state.refinedPartition))
for k, v := range state.refinedPartition {
    bestSuperPartition[k] = v
}
```

### 기존 leiden_state.go Seed=0 처리 패턴 (runOnce에서 그대로 활용)
```go
// Source: graph/leiden_state.go reset()
var src rand.Source
if seed != 0 {
    src = rand.NewSource(seed)
} else {
    src = rand.NewSource(time.Now().UnixNano())
}
st.rng = rand.New(src)
```

### multi-run 결과 반환 패턴 (신규)
```go
// baseSeed는 루프 전에 한 번만 계산
baseSeed := time.Now().UnixNano()
var bestResult CommunityResult
bestQ := math.Inf(-1)
var lastErr error
for i := 0; i < effectiveNumRuns; i++ {
    res, err := d.runOnce(g, baseSeed+int64(i))
    if err != nil {
        lastErr = err
        continue
    }
    if res.Modularity > bestQ {
        bestQ = res.Modularity
        bestResult = res
    }
}
if bestQ == math.Inf(-1) {
    return CommunityResult{}, lastErr
}
return bestResult, nil
```

## Environment Availability

Step 2.6: SKIPPED (no external dependencies — pure Go stdlib, no new packages)

## Open Questions

1. **runOnce helper 분리 여부**
   - What we know: CONTEXT.md에서 Claude's Discretion으로 지정됨
   - What's unclear: 인라인 루프 vs helper 분리 중 어느 쪽이 더 명료한지
   - Recommendation: helper 분리 (`runOnce`) 권장. `Detect`가 다중 책임(멀티런 결정 + 알고리즘 실행)을 갖지 않도록 분리하면 테스트와 코드 리뷰가 용이.

2. **TestLeidenStabilityMultiRun 배치 파일**
   - What we know: `leiden_test.go`(알고리즘 단위 테스트)와 `accuracy_test.go`(NMI/Q 정확도 테스트) 모두 후보
   - What's unclear: NMI threshold를 검증하므로 accuracy_test.go가 더 적합할 수 있음
   - Recommendation: `accuracy_test.go`에 배치. NMI threshold assertion이 포함된 테스트는 accuracy_test.go의 패턴과 일치.

## Sources

### Primary (HIGH confidence)
- `graph/leiden.go` — Detect 메서드 전체 구조, bestQ/bestSuperPartition 패턴 직접 확인
- `graph/leiden_state.go` — reset() Seed=0 처리 패턴, pool 패턴 직접 확인
- `graph/detector.go` — LeidenOptions struct 현재 구조 직접 확인
- `graph/leiden_test.go` — 기존 테스트 패턴 (NumRuns=1 명시 대상) 직접 확인
- `graph/accuracy_test.go` — NMI 테스트 패턴, stability 테스트 추가 위치 직접 확인
- `.planning/phases/01-leiden-nmi-seed/01-CONTEXT.md` — 모든 locked decisions 및 구현 세부사항

### Secondary (MEDIUM confidence)
- `.planning/STATE.md` — 기존 프로젝트 패턴(bestSuperPartition deep-copy, pool 패턴 등) 이력 확인

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — 외부 라이브러리 없음, 기존 stdlib 패턴만 사용
- Architecture: HIGH — 기존 코드를 직접 읽고 패턴 확인, CONTEXT.md decisions가 구체적
- Pitfalls: HIGH — 기존 코드의 pool/state/seed 패턴을 직접 분석하여 도출

**Research date:** 2026-03-30
**Valid until:** 2026-06-30 (stable Go stdlib patterns)
