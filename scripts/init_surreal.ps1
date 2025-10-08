Param(
  [string]$SurrealUrl = "http://127.0.0.1:8000",
  [string]$User = "root",
  [string]$Pass = "root",
  [string]$NS = "chaosmith",
  [string]$DB = "wims"
)

$schemaPath = Join-Path (Get-Location) "etc/schema.surql"
if (!(Test-Path $schemaPath)) { Write-Error "Schema file not found: $schemaPath"; exit 1 }

try {
  $pair = "{0}:{1}" -f $User, $Pass
  $b64 = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes($pair))
  $headers = @{ 'Authorization' = "Basic $b64"; 'NS' = $NS; 'DB' = $DB; 'Content-Type' = 'application/json'; 'Accept' = 'application/json' }
  $raw = Get-Content -Raw -Path $schemaPath
  $payload = @{ query = $raw }
  $body = $payload | ConvertTo-Json -Depth 2
  $resp = Invoke-RestMethod -Method Post -Uri "$SurrealUrl/sql" -Headers $headers -Body $body
  Write-Host ($resp | ConvertTo-Json -Depth 3)
  Write-Host ("Schema applied to {0} (ns={1} db={2})" -f $SurrealUrl,$NS,$DB)
} catch {
  Write-Error $_
  exit 1
}
