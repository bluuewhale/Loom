# loom — Go GraphRAG Library

## What This Is

`loom`은 Go로 개발된 고성능 오픈소스 GraphRAG 라이브러리입니다. LLM으로 추출한 지식 그래프나 기존 그래프 DB에서 읽어온 그래프에 대해 community detection, 중심성 분석, 경로 탐색 등 GraphRAG에 필요한 알고리즘을 제공합니다. 실시간 쿼리 환경에서 다수의 소규모 그래프를 병렬로 처리할 수 있는 것을 목표로 합니다.

## Core Value

개발자가 GraphRAG 파이프라인을 Go로 구현할 때 필요한 그래프 알고리즘을 교체 가능한 인터페이스로 빠르게 가져다 쓸 수 있어야 한다.

## Requirements

### Validated

- ✓ 가중치 유향/무향 그래프 자료구조 (`Graph`, `NodeID`, `Edge`) — Phase 01-01
- ✓ Newman-Girvan Modularity 계산 (`ComputeModularity`, `ComputeModularityWeighted`) — Phase 01-02
- ✓ 문자열 레이블 ↔ NodeID 변환 (`NodeRegistry`) — Phase 01-03
- ✓ 벤치마크 픽스처 (Karate Club 34노드, 78엣지, ground-truth partition) — Phase 01-02

### Active

*(Milestone 1: Community Detection)*

- [ ] `CommunityDetector` 인터페이스 — 알고리즘 교체 가능한 통합 진입점
- [ ] Louvain 알고리즘 구현 (phase 최적화, resolution parameter)
- [ ] Leiden 알고리즘 구현 (Louvain 개선판 — 커뮤니티 단절 방지)
- [ ] 10,000 노드 그래프 기준 < 100ms/그래프 성능 목표
- [ ] 동시 안전(concurrent-safe) 설계 — 실시간 쿼리 시 여러 그래프 병렬 분석
- [ ] Karate Club 포함 표준 벤치마크 그래프 픽스처 확장
- [ ] 정확도 검증: 알려진 그래프에서 ground-truth 커뮤니티 재현

### Out of Scope

- 그래프 DB 커넥터 (Neo4j, Memgraph 등) — I/O 레이어는 라이브러리 외부 책임
- LLM 연동 / 임베딩 — 알고리즘 레이어에 집중; LLM은 상위 레이어에서 처리
- 분산 처리 (멀티 머신) — 단일 프로세스 내 고루틴 병렬화가 현재 목표
- 시각화 — 그래프 렌더링은 외부 도구 영역
- 영속성 / 직렬화 — 순수 인메모리 라이브러리

## Context

**현재 코드베이스 상태:**
- `graph/graph.go`: `Graph` 자료구조, 가중치 엣지, 유향/무향 지원
- `graph/modularity.go`: modularity Q 계산 (커뮤니티 탐지 품질 지표)
- `graph/registry.go`: 문자열 노드명 ↔ `NodeID` 양방향 매핑
- `graph/testdata/karate.go`: 34노드 Karate Club (알고리즘 검증 기준)
- 외부 의존성 없음 — 순수 표준 라이브러리

**GraphRAG 맥락:**
- GraphRAG는 문서에서 추출한 엔티티/관계 그래프를 RAG 검색에 활용하는 방법론
- community detection은 그래프를 의미 단위로 묶어 청킹/요약/인덱싱에 사용
- 실시간 쿼리: 쿼리마다 서브그래프를 동적으로 구성 후 즉시 분석

**목표 사용자:**
- Go로 GraphRAG 파이프라인을 직접 구현하는 개발자
- 오픈소스 — 외부 기여 고려한 API 설계 필요

## Constraints

- **언어**: Go 1.26+ — 생태계 일관성, CGO 없음
- **의존성**: 최소화 — 외부 패키지 추가는 신중히 결정
- **동시성**: 고루틴 기반 병렬화 — `sync.Pool`, 채널, 워크풀 패턴 활용
- **API**: 인터페이스 기반 — 알고리즘은 교체 가능해야 함 (`CommunityDetector`)
- **테스트**: 알고리즘 정확도는 ground-truth 그래프로 검증, 성능은 벤치마크로 측정

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| 단일 패키지 (`package graph`) | 초기엔 단순하게, 필요 시 분리 | — Pending |
| `map[NodeID]int` as Partition | 외부 타입 없이 표현 가능, zero-alloc 교체 쉬움 | — Pending |
| `NodeRegistry` 선택적 사용 | 정수 ID 직접 사용하는 성능 우선 경로 유지 | — Pending |
| `CommunityDetector` 인터페이스 | 알고리즘 교체 가능성 + 테스트 용이성 | — Pending |

## Evolution

이 문서는 마일스톤 전환 시 업데이트됩니다.

**각 페이즈 완료 후 (`/gsd:transition`):**
1. 완료된 요구사항 → Validated로 이동 (페이즈 참조 포함)
2. 무효화된 요구사항 → Out of Scope로 이동 (이유 기록)
3. 새로 도출된 요구사항 → Active에 추가
4. 주요 결정사항 → Key Decisions에 기록

**각 마일스톤 완료 후 (`/gsd:complete-milestone`):**
1. 전체 섹션 검토
2. Core Value 여전히 유효한가?
3. Out of Scope 항목 — 이유 여전히 유효한가?
4. 다음 마일스톤 Active 요구사항 갱신

---
*Last updated: 2026-03-29 after Phase 02 (Interface + Louvain Core) — CommunityDetector interface, LouvainOptions, LeidenOptions, CommunityResult, and full Louvain algorithm complete. Q=0.4156 on Karate Club. IFACE-01~06, LOUV-01~05 validated.*
