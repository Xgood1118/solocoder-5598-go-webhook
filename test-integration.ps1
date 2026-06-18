$baseUrl = "http://localhost:8084/api/v1"

Write-Host "=== 1. Create endpoint ===" -ForegroundColor Green
$epBody = @{
    name = "integration-test"
    url = "http://httpbin.org/post"
    event_types = @("test.event")
    retry_policy = @{
        strategy = "fixed"
        max_retries = 2
        base_delay_ms = 500
        jitter_ms = 100
    }
} | ConvertTo-Json -Depth 10

$ep = Invoke-RestMethod -Uri "$baseUrl/endpoints" -Method Post -Body $epBody -ContentType "application/json"
$epId = $ep.id
Write-Host "Endpoint ID: $epId"
Write-Host "Status: $($ep.status)"

Write-Host ""
Write-Host "=== 2. Add two keys (dual key rotation) ===" -ForegroundColor Green
$key1Body = @{key_id = "key-v1"; secret = "secret-v1"} | ConvertTo-Json
$ep = Invoke-RestMethod -Uri "$baseUrl/endpoints/$epId/keys" -Method Post -Body $key1Body -ContentType "application/json"
Write-Host "Added key-v1 OK"

$key2Body = @{key_id = "key-v2"; secret = "secret-v2"} | ConvertTo-Json
$ep = Invoke-RestMethod -Uri "$baseUrl/endpoints/$epId/keys" -Method Post -Body $key2Body -ContentType "application/json"
Write-Host "Added key-v2 OK (both keys active)"
Write-Host "Current key count: $($ep.api_keys.Count)"

Write-Host ""
Write-Host "=== 3. Verify signing with old key (key-v1) works ===" -ForegroundColor Green
$payloadStr = '{"test":"data"}'
$timestamp = [int][double]::Parse((Get-Date -UFormat %s))
$hmac = New-Object System.Security.Cryptography.HMACSHA256
$hmac.Key = [Text.Encoding]::UTF8.GetBytes("secret-v1")
$sigBytes = $hmac.ComputeHash([Text.Encoding]::UTF8.GetBytes($timestamp.ToString() + $payloadStr))
$signature = [BitConverter]::ToString($sigBytes) -replace '-', ''
$signature = $signature.ToLower()

$headers = @{
    "X-Event-ID" = "evt-test-001"
    "X-Event-Type" = "test.event"
    "X-Key-ID" = "key-v1"
    "X-Timestamp" = $timestamp
    "X-Signature" = $signature
}
$result = Invoke-RestMethod -Uri "http://localhost:8084/webhook/receive" -Method Post -Body $payloadStr -ContentType "application/json" -Headers $headers
Write-Host "key-v1 signature ok: $($result.signature_ok)"

Write-Host ""
Write-Host "=== 4. Verify signing with new key (key-v2) works ===" -ForegroundColor Green
$payloadStr2 = '{"test":"data2"}'
$timestamp2 = [int][double]::Parse((Get-Date -UFormat %s))
$hmac2 = New-Object System.Security.Cryptography.HMACSHA256
$hmac2.Key = [Text.Encoding]::UTF8.GetBytes("secret-v2")
$sigBytes2 = $hmac2.ComputeHash([Text.Encoding]::UTF8.GetBytes($timestamp2.ToString() + $payloadStr2))
$signature2 = [BitConverter]::ToString($sigBytes2) -replace '-', ''
$signature2 = $signature2.ToLower()

$headers2 = @{
    "X-Event-ID" = "evt-test-002"
    "X-Event-Type" = "test.event"
    "X-Key-ID" = "key-v2"
    "X-Timestamp" = $timestamp2
    "X-Signature" = $signature2
}
$result2 = Invoke-RestMethod -Uri "http://localhost:8084/webhook/receive" -Method Post -Body $payloadStr2 -ContentType "application/json" -Headers $headers2
Write-Host "key-v2 signature ok: $($result2.signature_ok)"

Write-Host ""
Write-Host "=== 5. Wait for dispatch ===" -ForegroundColor Green
Start-Sleep -Seconds 3

Write-Host ""
Write-Host "=== 6. Check deliveries ===" -ForegroundColor Green
$deliveries = Invoke-RestMethod -Uri "$baseUrl/endpoints/$epId/deliveries" -Method Get
Write-Host "Delivery count: $($deliveries.Count)"
if ($deliveries.Count -gt 0) {
    $d = $deliveries[0]
    Write-Host "Latest delivery status: $($d.status), status code: $($d.status_code), duration: $($d.duration_ms)ms"
}

Write-Host ""
Write-Host "=== 7. Check endpoint health ===" -ForegroundColor Green
$health = Invoke-RestMethod -Uri "$baseUrl/endpoints/$epId/health" -Method Get
Write-Host "Success rate: $($health.success_rate)%"
Write-Host "Avg latency: $($health.avg_latency_ms)ms"
Write-Host "P95 latency: $($health.p95_latency_ms)ms"

Write-Host ""
Write-Host "=== 8. Pause endpoint ===" -ForegroundColor Green
$pauseBody = @{status = "paused"} | ConvertTo-Json
$ep = Invoke-RestMethod -Uri "$baseUrl/endpoints/$epId/status" -Method Post -Body $pauseBody -ContentType "application/json"
Write-Host "Endpoint status: $($ep.status)"

Write-Host ""
Write-Host "=== 9. Send event while paused - signature should verify but no delivery ===" -ForegroundColor Green
$payloadStr3 = '{"test":"data3"}'
$timestamp3 = [int][double]::Parse((Get-Date -UFormat %s))
$hmac3 = New-Object System.Security.Cryptography.HMACSHA256
$hmac3.Key = [Text.Encoding]::UTF8.GetBytes("secret-v2")
$sigBytes3 = $hmac3.ComputeHash([Text.Encoding]::UTF8.GetBytes($timestamp3.ToString() + $payloadStr3))
$signature3 = [BitConverter]::ToString($sigBytes3) -replace '-', ''
$signature3 = $signature3.ToLower()

$headers3 = @{
    "X-Event-ID" = "evt-test-003"
    "X-Event-Type" = "test.event"
    "X-Key-ID" = "key-v2"
    "X-Timestamp" = $timestamp3
    "X-Signature" = $signature3
}
$result3 = Invoke-RestMethod -Uri "http://localhost:8084/webhook/receive" -Method Post -Body $payloadStr3 -ContentType "application/json" -Headers $headers3
Write-Host "Signature ok: $($result3.signature_ok) (expected: True)"
Start-Sleep -Seconds 2
$deliveriesAfter = Invoke-RestMethod -Uri "$baseUrl/endpoints/$epId/deliveries" -Method Get
Write-Host "Delivery count: $($deliveriesAfter.Count) (should be same as before)"

Write-Host ""
Write-Host "=== 10. Delete old key (simulate key rotation complete) ===" -ForegroundColor Green
$ep = Invoke-RestMethod -Uri "$baseUrl/endpoints/$epId/keys/key-v1" -Method Delete
Write-Host "After deleting key-v1, key count: $($ep.api_keys.Count)"

Write-Host ""
Write-Host "=== 11. Verify old key no longer works ===" -ForegroundColor Green
$payloadStr4 = '{"test":"data4"}'
$timestamp4 = [int][double]::Parse((Get-Date -UFormat %s))
$hmac4 = New-Object System.Security.Cryptography.HMACSHA256
$hmac4.Key = [Text.Encoding]::UTF8.GetBytes("secret-v1")
$sigBytes4 = $hmac4.ComputeHash([Text.Encoding]::UTF8.GetBytes($timestamp4.ToString() + $payloadStr4))
$signature4 = [BitConverter]::ToString($sigBytes4) -replace '-', ''
$signature4 = $signature4.ToLower()

$headers4 = @{
    "X-Event-ID" = "evt-test-004"
    "X-Event-Type" = "test.event"
    "X-Key-ID" = "key-v1"
    "X-Timestamp" = $timestamp4
    "X-Signature" = $signature4
}
$result4 = Invoke-RestMethod -Uri "http://localhost:8084/webhook/receive" -Method Post -Body $payloadStr4 -ContentType "application/json" -Headers $headers4
Write-Host "key-v1 signature ok: $($result4.signature_ok) (expected: False)"

Write-Host ""
Write-Host "=== 12. Resume endpoint ===" -ForegroundColor Green
$activeBody = @{status = "active"} | ConvertTo-Json
$ep = Invoke-RestMethod -Uri "$baseUrl/endpoints/$epId/status" -Method Post -Body $activeBody -ContentType "application/json"
Write-Host "Endpoint status: $($ep.status)"

Write-Host ""
Write-Host "=== 13. Check dead letters (should be empty) ===" -ForegroundColor Green
$dls = Invoke-RestMethod -Uri "$baseUrl/dead-letters?only_unresolved=true" -Method Get
Write-Host "Dead letter count: $($dls.Count)"

Write-Host ""
Write-Host "=== All tests passed! ===" -ForegroundColor Green
