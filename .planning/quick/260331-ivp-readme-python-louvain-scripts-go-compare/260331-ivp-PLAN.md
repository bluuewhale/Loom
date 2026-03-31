---
phase: quick
plan: 260331-ivp
type: execute
wave: 1
depends_on: []
files_modified:
  - scripts/go-compare/go.mod
  - scripts/go-compare/main.go
  - README.md
autonomous: true
must_haves:
  truths:
    - "scripts/go-compare/ contains a standalone Go module that benchmarks gonum Louvain"
    - "README Performance table includes gonum comparison rows with real timing numbers"
    - "README python-louvain footnote clarifies library name and usage"
  artifacts:
    - path: "scripts/go-compare/main.go"
      provides: "Go benchmark comparing loom vs gonum community detection"
    - path: "README.md"
      provides: "Updated Performance table with gonum rows and clarified python-louvain footnote"
---

<objective>
Add a Go benchmark comparison script (scripts/go-compare/) that times gonum's Louvain on the same 10K-node graph as loom. Update README Performance table with real numbers from the benchmark run, and fix the python-louvain footnote to clarify library name and usage.
</objective>

<tasks>

<task type="auto">
  <name>Task 1: Create scripts/go-compare/ Go benchmark module</name>
  <files>scripts/go-compare/go.mod, scripts/go-compare/main.go</files>
  <action>
Create standalone Go module that generates a random 10K-node graph and benchmarks gonum community detection, then run it to get real timing numbers.
  </action>
  <done>go-compare module created, dependencies fetched, benchmark runs and produces timing output.</done>
</task>

<task type="auto">
  <name>Task 2: Update README Performance table and python-louvain footnote</name>
  <files>README.md</files>
  <action>
Add gonum comparison rows to Performance table using real numbers from Task 1. Update python-louvain footnote to clarify the library name (python-louvain, imported as community) and usage context.
  </action>
  <done>README Performance table has gonum rows. python-louvain footnote is accurate.</done>
</task>

</tasks>

<output>
After completion, create `.planning/quick/260331-ivp-readme-python-louvain-scripts-go-compare/260331-ivp-SUMMARY.md`
</output>
