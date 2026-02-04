# PowerShell Client Configuration Generator for paqet
# Run as Administrator

param(
  [string]$DefaultOutput = "client.config.yaml"
)

# --- Colors & Styling Helpers ---
# Using ASCII characters instead of Unicode to avoid encoding issues on different Windows locales

function Write-Header {
    param([string]$Msg)
    Write-Host "`n=== $Msg ===" -ForegroundColor Cyan
}

function Write-Step {
    param([string]$Msg)
    Write-Host "`n>> $Msg" -ForegroundColor Blue
}

function Write-Info {
    param([string]$Msg)
    Write-Host " [i] $Msg" -ForegroundColor Cyan
}

function Write-Success {
    param([string]$Msg)
    Write-Host " [+] $Msg" -ForegroundColor Green
}

function Write-Warn {
    param([string]$Msg)
    Write-Host " [!] $Msg" -ForegroundColor Yellow
}

function Write-ErrorMsg {
    param([string]$Msg)
    Write-Host " [x] $Msg" -ForegroundColor Red
}

# --- Security Check ---
function Test-Administrator {
  $principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
  return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

if (-not (Test-Administrator)) {
    Write-Warn "This script is not running as Administrator. Network detection (GUIDs/MACs) may fail or be incomplete."
}

# --- Input Helpers ---

function Write-Prompt {
    param(
        [string]$Label,
        [string]$DefaultValue = $null
    )
    
    if (-not [string]::IsNullOrWhiteSpace($DefaultValue)) {
        Write-Host -NoNewline "${Label} "
        Write-Host -NoNewline "[${DefaultValue}]" -ForegroundColor Yellow
        Write-Host -NoNewline ": "
        $input = Read-Host
        if ([string]::IsNullOrWhiteSpace($input)) { return $DefaultValue }
        return $input
    } else {
        Write-Host -NoNewline "${Label}: "
        return (Read-Host)
    }
}

# --- Validation Helpers ---

function Test-IPv4 {
  param([string]$Value)
  $octets = $Value -split '\.'
  if ($octets.Count -ne 4) { return $false }
  foreach ($o in $octets) {
    if ($o -notmatch '^\d+$') { return $false }
    $n = [int]$o
    if ($n -lt 0 -or $n -gt 255) { return $false }
  }
  return $true
}

function Test-Mac {
  param([string]$Value)
  return $Value -match '^([0-9a-fA-F]{2}[:-]){5}[0-9a-fA-F]{2}$'
}

function Test-HostPort {
  param([string]$Value)
  if ($Value -match '^.+:(\d+)$') {
    $port = [int]$Matches[1]
    return ($port -ge 1 -and $port -le 65535)
  }
  return $false
}

# --- Network Helpers ---

function Get-PreferredIPv4 {
  param([int]$IfIndex)
  $ip = Get-NetIPAddress -InterfaceIndex $IfIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
    Where-Object { $_.IPAddress -notlike '169.254*' -and $_.AddressState -eq 'Preferred' } |
    Select-Object -First 1 -ExpandProperty IPAddress
  if (-not $ip) {
    # Fallback
    $ip = Get-NetIPAddress -InterfaceIndex $IfIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue |
      Where-Object { $_.IPAddress -notlike '169.254*' } |
      Select-Object -First 1 -ExpandProperty IPAddress
  }
  return $ip
}

function Get-GatewayIPv4 {
  param([int]$IfIndex)
  $cfg = Get-NetIPConfiguration -InterfaceIndex $IfIndex -ErrorAction SilentlyContinue
  if ($cfg -and $cfg.IPv4DefaultGateway) {
    return $cfg.IPv4DefaultGateway.NextHop
  }
  return $null
}

function Get-GatewayMac {
  param(
    [int]$IfIndex,
    [string]$Gateway
  )
  if ([string]::IsNullOrWhiteSpace($Gateway)) { return $null }
  
  # Try basic neighbor lookup
  $neighbor = Get-NetNeighbor -InterfaceIndex $IfIndex -IPAddress $Gateway -ErrorAction SilentlyContinue | Select-Object -First 1
  
  # If not found, try pinging then lookup again
  if (-not $neighbor -or $neighbor.State -eq 'Unreachable') {
    Test-Connection -Count 1 -Quiet $Gateway | Out-Null
    Start-Sleep -Milliseconds 200
    $neighbor = Get-NetNeighbor -InterfaceIndex $IfIndex -IPAddress $Gateway -ErrorAction SilentlyContinue | Select-Object -First 1
  }
  
  if ($neighbor) { return ($neighbor.LinkLayerAddress -replace '-', ':') }
  return $null
}

# --- Main Logic ---

Write-Header "paqet Client Configuration"

# 1. Server Connection Info
Write-Step "Step 1: Connection Settings"
while ($true) {
    $serverAddr = Write-Prompt "Server Address (IP:Port)"
    if (Test-HostPort $serverAddr) { break }
    Write-Warn "Invalid format. Use IP:Port (e.g., 203.0.113.10:9999)"
}
Write-Info "Target Server: $serverAddr"



# 2. Network Selection
Write-Step "Step 2: Network Interface"

$configs = Get-NetIPConfiguration -ErrorAction SilentlyContinue | Where-Object {
  $_.NetAdapter -and $_.NetAdapter.Status -eq "Up"
}

if (-not $configs -or $configs.Count -eq 0) {
    Write-ErrorMsg "No active network interfaces found."
    exit 1
}

$candidates = $configs
$selectedConfig = $null

if ($candidates.Count -eq 1) {
    $selectedConfig = $candidates[0]
    Write-Info "Auto-selected only interface: $($selectedConfig.NetAdapter.Name)"
} else {
    Write-Host "Available interfaces:"
    for ($i = 0; $i -lt $candidates.Count; $i++) {
        $adapter = $candidates[$i].NetAdapter
        $ip = Get-PreferredIPv4 -IfIndex $adapter.ifIndex
        $name = $adapter.Name
        if ($ip) {
            Write-Host "  [$($i+1)] $name (IP: $ip)"
        } else {
            Write-Host "  [$($i+1)] $name"
        }
    }
    
    while ($true) {
        $choice = Write-Prompt "Select interface" "1"
        if ($choice -match '^\d+$') {
            $idx = [int]$choice
            if ($idx -ge 1 -and $idx -le $candidates.Count) { 
                $selectedConfig = $candidates[$idx-1]
                break 
            }
        }
        Write-Warn "Invalid selection."
    }
}

$selected = $selectedConfig.NetAdapter
$interfaceName = $selected.Name
$guidClean = $selected.InterfaceGuid -replace '{|}'
$npcapGuid = "\Device\NPF_{$guidClean}"
Write-Info "Using Interface: $interfaceName"
# Write-Info "GUID: $npcapGuid"

# 3. Discovery
Write-Step "Step 3: Network Discovery"
Write-Info "Detecting network details..."

$ifaceIp = Get-PreferredIPv4 -IfIndex $selected.ifIndex
if ([string]::IsNullOrWhiteSpace($ifaceIp)) {
    Write-Warn "Could not detect local IP."
    while ($true) {
        $ifaceIp = Write-Prompt "Enter Local IPv4 Address"
        if (Test-IPv4 $ifaceIp) { break }
        Write-Warn "Invalid IPv4."
    }
} else {
    Write-Success "Detected Local IP: $ifaceIp"
}
$clientIp = $ifaceIp
$clientPort = "0" # Random port for client

$gatewayIp = Get-GatewayIPv4 -IfIndex $selected.ifIndex
$routerMac = Get-GatewayMac -IfIndex $selected.ifIndex -Gateway $gatewayIp

if (-not [string]::IsNullOrWhiteSpace($routerMac)) {
    Write-Success "Detected Router MAC: $routerMac"
} else {
    Write-Warn "Could not detect Router MAC for gateway $gatewayIp"
    while ($true) {
         $routerMac = Write-Prompt "Enter Router MAC Address"
         if (Test-Mac $routerMac) { 
             $routerMac = $routerMac -replace '-', ':'
             break 
         }
         Write-Warn "Invalid MAC format."
    }
}

# 4. Encryption
Write-Step "Step 4: Encryption"
Write-Warn "Note: The key MUST match the exact key generated on your server."

$kcpKey = ""
while ([string]::IsNullOrWhiteSpace($kcpKey)) {
    $kcpKey = Write-Prompt "Enter Server KCP Key"
    if ([string]::IsNullOrWhiteSpace($kcpKey)) {
        Write-Warn "Key cannot be empty."
    }
}
Write-Success "Key set."

# 5. Client Settings
Write-Step "Step 5: Client Settings"
while ($true) {
    $socksPort = Write-Prompt "Local SOCKS5 Listen Port" "1080"
    if ($socksPort -match '^\d+$' -and [int]$socksPort -ge 1 -and [int]$socksPort -le 65535) {
        break
    }
    Write-Warn "Invalid port."
}
$socksListen = "127.0.0.1:$socksPort"

# 6. Output
Write-Step "Step 6: Finalizing"
$outputFile = Write-Prompt "Output configuration file" $DefaultOutput

$yaml = @"
role: "client"
log:
  level: "info"
socks5:
  - listen: "$socksListen"
network:
  interface: "$interfaceName"
  guid: '$npcapGuid'
  ipv4:
    addr: "${clientIp}:${clientPort}"
    router_mac: "$routerMac"
  tcp:
    local_flag: ["PA"]
    remote_flag: ["PA"]
server:
  addr: "$serverAddr"
transport:
  protocol: "kcp"
  conn: 1
  kcp:
    mode: "fast"
    key: "$kcpKey"
"@

$yaml | Set-Content -Path $outputFile -Encoding utf8
Write-Success "Configuration saved to $outputFile"

Write-Header "Setup Complete!"
Write-Info "You can now run the client with:"
Write-Host "  paqet run -c $outputFile" -ForegroundColor Green
Write-Info "Make sure you have Npcap installed!"
