$payload = '{"user_id":123,"name":"test user"}'
$secret = "my-secret-key-123"
$timestamp = [int][double]::Parse((Get-Date -UFormat %s))
$tsStr = $timestamp.ToString()

$hmacsha = New-Object System.Security.Cryptography.HMACSHA256
$hmacsha.Key = [Text.Encoding]::UTF8.GetBytes($secret)
$bytes = [Text.Encoding]::UTF8.GetBytes($tsStr + $payload)
$hash = $hmacsha.ComputeHash($bytes)
$signature = [BitConverter]::ToString($hash) -replace '-', ''
$signature = $signature.ToLower()

Write-Host "Timestamp: $timestamp"
Write-Host "Signature: $signature"

$headers = @{
    "X-Event-ID" = "evt-test-002"
    "X-Event-Type" = "user.created"
    "X-Key-ID" = "key-1"
    "X-Timestamp" = $timestamp
    "X-Signature" = $signature
}

$result = Invoke-RestMethod -Uri "http://localhost:8084/webhook/receive" -Method Post -Body $payload -ContentType "application/json" -Headers $headers
$result | ConvertTo-Json
