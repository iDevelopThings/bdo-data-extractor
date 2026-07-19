#Requires -Version 5.1
<#
.SYNOPSIS
  A/B wall-clock timing for the JSON `build` pipeline (no icons/worldmap).

.DESCRIPTION
  Conventions:
  - Only runs `build` — never icons / knowledge-icons / worldmap (~1GB).
  - Compares GOMAXPROCS=1 (mostly serial) vs default parallelism.
  - Parses stable `[done] <stage>  <secs>s  (total <secs>s)` lines from the build log.
  - Writes per-run logs under .tmp/bench/<stamp>/ and a stages.csv summary.

  Related:
  - Microbench: go test ./internal/tables/ -bench=BenchmarkDecodeItemStats -benchmem -count=10
  - CPU profile: add -CpuProfile .tmp/bench/cpu.pprof then `go tool pprof` that file.
  - Embedder path: pipeline.Options{SkipAssets: true} skips icons/worldmap in RunAll.

.EXAMPLE
  .\scripts\bench-build.ps1 -Runs 5 -Region na -Pretty
#>
param(
  [int]$Runs = 3,
  [string]$Region = "na",
  [string]$Lang = "en",
  [string]$GameDir = "",
  [string]$OutRoot = ".tmp/bench",
  [switch]$Pretty,
  [string]$CpuProfile = "", # written on the first parallel run only
  [string]$Bin = ".tmp/bdo-data-extractor.exe"
)

$ErrorActionPreference = "Stop"
Set-Location (Split-Path $PSScriptRoot -Parent)

function Median([double[]]$vals) {
  if ($vals.Count -eq 0) { return 0 }
  $sorted = $vals | Sort-Object
  $mid = [int][math]::Floor(($sorted.Count - 1) / 2)
  if ($sorted.Count % 2 -eq 1) { return [double]$sorted[$mid] }
  return ([double]$sorted[$mid] + [double]$sorted[$mid + 1]) / 2
}

Write-Host "building binary -> $Bin"
New-Item -ItemType Directory -Force -Path (Split-Path $Bin -Parent) | Out-Null
go build -o $Bin .
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$runRoot = Join-Path $OutRoot $stamp
New-Item -ItemType Directory -Force -Path $runRoot | Out-Null

$modes = @(
  @{ Name = "serial";   Env = @{ GOMAXPROCS = "1" } },
  @{ Name = "parallel"; Env = @{} }
)

$stageTotals = @{} # key -> list of stage seconds
$buildTotals = @{ serial = New-Object System.Collections.Generic.List[double]; parallel = New-Object System.Collections.Generic.List[double] }

foreach ($mode in $modes) {
  foreach ($i in 1..$Runs) {
    $label = "{0}-run{1}" -f $mode.Name, $i
    $outDir = Join-Path $runRoot $label
    $logPath = Join-Path $runRoot "$label.log"
    New-Item -ItemType Directory -Force -Path $outDir | Out-Null

    $args = @("build", "--region=$Region", "--lang=$Lang", "--out=$outDir")
    if ($Pretty) { $args += "--pretty" }
    if ($GameDir) { $args += "--game=$GameDir" }
    if ($CpuProfile -and $mode.Name -eq "parallel" -and $i -eq 1) {
      $args += "--cpuprofile=$CpuProfile"
    }

    Write-Host ("`n=== {0} ===" -f $label)
    foreach ($k in $mode.Env.Keys) {
      Set-Item -Path "Env:$k" -Value $mode.Env[$k]
    }
    if ($mode.Name -eq "parallel") {
      Remove-Item Env:GOMAXPROCS -ErrorAction SilentlyContinue
    }

    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    & $Bin @args 2>&1 | Tee-Object -FilePath $logPath
    if ($LASTEXITCODE -ne 0) { throw "build failed ($label), see $logPath" }
    $sw.Stop()
    $buildTotals[$mode.Name].Add($sw.Elapsed.TotalSeconds)

    Get-Content $logPath | ForEach-Object {
      if ($_ -match '^\[done\] (.+?)  ([0-9.]+)s  \(total ([0-9.]+)s\)') {
        $stage = $Matches[1].Trim()
        $secs = [double]$Matches[2]
        $key = "{0}|{1}" -f $mode.Name, $stage
        if (-not $stageTotals.ContainsKey($key)) {
          $stageTotals[$key] = New-Object System.Collections.Generic.List[double]
        }
        $stageTotals[$key].Add($secs)
      }
    }
  }
}

$csv = Join-Path $runRoot "stages.csv"
$rows = @("mode,stage,median_s,min_s,max_s,n")
foreach ($key in ($stageTotals.Keys | Sort-Object)) {
  $mode, $stage = $key.Split("|", 2)
  $vals = @($stageTotals[$key])
  $rows += ("{0},{1},{2:N3},{3:N3},{4:N3},{5}" -f $mode, $stage, (Median $vals), ($vals | Measure-Object -Minimum).Minimum, ($vals | Measure-Object -Maximum).Maximum, $vals.Count)
}
$rows | Set-Content -Path $csv -Encoding utf8

Write-Host "`n=== summary (median wall clock, $Runs runs) ==="
foreach ($mode in @("serial", "parallel")) {
  $med = Median @($buildTotals[$mode])
  Write-Host ("  {0,-10} build: {1:N3}s" -f $mode, $med)
}
Write-Host "logs + stages.csv -> $runRoot"
if ($CpuProfile) {
  Write-Host "cpu profile -> $CpuProfile  (go tool pprof $CpuProfile)"
}
