param(
    [string]$ApiBaseUrl = 'http://127.0.0.1:18080',
    [string]$KubeContext = 'kubetask-p21',
    [string]$Image = 'busybox:1.37.0',
    [int]$TimeoutSeconds = 300,
    [switch]$KeepResources
)

# Run against a KubeTask server connected to the named minikube context.
# No status injection: Kubernetes Jobs and Pods execute the commands below.
$ErrorActionPreference = 'Stop'
$taskPrefix = 'p21-' + (Get-Date -Format 'MMddHHmmss') + '-' + [guid]::NewGuid().ToString('N').Substring(0, 4)
$taskCreatedWorkflows = [System.Collections.Generic.List[string]]::new()
$taskCreatedJobs = [System.Collections.Generic.List[string]]::new()
$taskResults = [System.Collections.Generic.List[object]]::new()
$ApiBaseUrl = $ApiBaseUrl.TrimEnd('/')

function Assert-Check([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}

function Invoke-KubectlJson([string[]]$Arguments) {
    $taskOutput = & kubectl --context $KubeContext --request-timeout=20s @Arguments
    if ($LASTEXITCODE -ne 0) { throw "kubectl failed: $($Arguments -join ' ')" }
    return ($taskOutput -join "`n") | ConvertFrom-Json
}

function New-Node([string]$Name, [string[]]$Dependencies, [string]$Command, [bool]$FailureBranch = $false) {
    return @{
        name = $Name
        dependsOn = @($Dependencies)
        runOnFailure = $FailureBranch
        taskSpec = @{
            type = 'OneTime'
            image = $Image
            command = @('sh', '-c', $Command)
            backoffLimit = 0
            activeDeadlineSeconds = 90
            ttlSecondsAfterFinished = 600
            resources = @{
                requests = @{cpu = '10m'; memory = '16Mi'}
                limits = @{cpu = '100m'; memory = '64Mi'}
            }
        }
    }
}

function Wait-Workflow([string]$Name, [string]$ExpectedPhase) {
    $taskDeadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        $taskDetail = Invoke-RestMethod -Uri "$ApiBaseUrl/api/v1/workflows/$Name" -TimeoutSec 20
        $taskRunning = @($taskDetail.graph.nodes | Where-Object phase -eq 'Running').Count
        Assert-Check ($taskRunning -le 2) "maxParallel exceeded for $Name : $taskRunning"
        if ($taskDetail.workflow.status.phase -in @('Succeeded', 'Failed')) {
            Assert-Check ($taskDetail.workflow.status.phase -eq $ExpectedPhase) "Unexpected workflow result for $Name : $($taskDetail.workflow.status.phase)"
            return $taskDetail
        }
        Start-Sleep -Seconds 1
    } while ((Get-Date) -lt $taskDeadline)
    $taskPods = Invoke-KubectlJson -Arguments @('get', 'pods', '-n', 'default', '-o', 'json')
    $taskSummary = $taskPods.items | Select-Object @{n = 'name'; e = {$_.metadata.name}}, @{n = 'phase'; e = {$_.status.phase}}
    throw "Workflow $Name timed out. Last detail: $($taskDetail | ConvertTo-Json -Depth 12 -Compress). Pods: $($taskSummary | ConvertTo-Json -Compress)"
}

function Test-Workflow([bool]$FailLint) {
    $taskCase = if ($FailLint) { 'fail' } else { 'pass' }
    $taskName = "$taskPrefix-$taskCase"
    $taskLintCommand = if ($FailLint) { 'echo SMOKE-lint; exit 7' } else { 'echo SMOKE-lint; sleep 2' }
    $taskBody = @{
        apiVersion = 'kubetask.kubetask.io/v1'
        kind = 'Workflow'
        metadata = @{name = $taskName; labels = @{'kubetask.io/smoke-run' = $taskPrefix}}
        spec = @{
            maxParallel = 2
            tasks = @(
                (New-Node 'build' @() 'echo SMOKE-build; sleep 3'),
                (New-Node 'test' @('build') 'echo SMOKE-test; sleep 5'),
                (New-Node 'lint' @('build') $taskLintCommand),
                (New-Node 'deploy' @('test', 'lint') 'echo SMOKE-deploy'),
                (New-Node 'notify' @('test', 'lint') 'echo SMOKE-notify' $true)
            )
        }
    }
    Write-Host "Creating $taskName through HTTP API"
    $taskCreated = Invoke-RestMethod -Method Post -Uri "$ApiBaseUrl/api/v1/workflows" -ContentType 'application/json' -Body ($taskBody | ConvertTo-Json -Depth 15) -TimeoutSec 20
    $taskCreatedWorkflows.Add($taskName)
    $taskClusterObject = Invoke-KubectlJson -Arguments @('get', 'workflow', $taskName, '-o', 'json')
    Assert-Check ($taskCreated.metadata.uid -eq $taskClusterObject.metadata.uid) 'API and kubectl point to different clusters'

    $taskExpected = if ($FailLint) { 'Failed' } else { 'Succeeded' }
    $taskDetail = Wait-Workflow $taskName $taskExpected
    Assert-Check ($taskDetail.graph.nodes.Count -eq 5) 'Expected all five DAG nodes'
    Assert-Check ($taskDetail.graph.edges.Count -eq 6) 'Expected all six dependency edges'
    $taskPhases = @{}
    foreach ($taskNode in $taskDetail.graph.nodes) { $taskPhases[$taskNode.name] = $taskNode.phase }
    foreach ($taskNodeName in @('build', 'test')) { Assert-Check ($taskPhases[$taskNodeName] -eq 'Succeeded') "$taskNodeName must succeed" }
    if ($FailLint) {
        Assert-Check ($taskPhases.lint -eq 'Failed' -and $taskPhases.deploy -eq 'Skipped' -and $taskPhases.notify -eq 'Succeeded') 'Failure propagation or handler result is incorrect'
    } else {
        Assert-Check ($taskPhases.lint -eq 'Succeeded' -and $taskPhases.deploy -eq 'Succeeded' -and $taskPhases.notify -eq 'Skipped') 'Success path or skipped handler result is incorrect'
    }

    $taskPodsChecked = 0
    foreach ($taskNode in $taskDetail.graph.nodes) {
        if ($taskNode.phase -eq 'Skipped') {
            Assert-Check (-not $taskNode.taskName) 'Skipped node must not have an executed child Task'
            continue
        }
        $taskJobs = Invoke-KubectlJson -Arguments @('get', 'jobs', '-n', 'default', '-l', "kubetask.io/task=$($taskNode.taskName)", '-o', 'json')
        Assert-Check ($taskJobs.items.Count -eq 1) "Expected one real Job for $($taskNode.name)"
        $taskJob = $taskJobs.items[0]
        $taskCreatedJobs.Add($taskJob.metadata.name)
        $taskPods = Invoke-KubectlJson -Arguments @('get', 'pods', '-n', 'default', '-l', "job-name=$($taskJob.metadata.name)", '-o', 'json')
        Assert-Check ($taskPods.items.Count -eq 1) "Expected one real Pod with backoffLimit=0 for $($taskNode.name)"
        $taskPod = $taskPods.items[0]
        Assert-Check ($taskPod.status.phase -eq $taskNode.phase) "Pod result differs from DAG node $($taskNode.name)"
        $taskContainer = $taskPod.status.containerStatuses | Where-Object name -eq 'task'
        $taskExitCode = if ($FailLint -and $taskNode.name -eq 'lint') { 7 } else { 0 }
        Assert-Check ($taskContainer.state.terminated.exitCode -eq $taskExitCode) "Unexpected container exit code for $($taskNode.name)"
        $taskSse = Invoke-WebRequest -Uri "$ApiBaseUrl/api/v1/tasks/$($taskNode.taskName)/logs?tail=100&follow=false" -TimeoutSec 20
        Assert-Check (($taskSse.Headers['Content-Type'] -join ';') -like 'text/event-stream*') 'Log endpoint did not return SSE'
        Assert-Check ($taskSse.Content -match "data: SMOKE-$($taskNode.name)") "Actual Pod logs missing for $($taskNode.name)"
        $taskPodsChecked++
    }
    Assert-Check ($taskPodsChecked -eq 4) 'Expected four executed Pods per workflow'
    $taskResults.Add([PSCustomObject]@{name = $taskName; phase = $taskExpected; nodes = $taskPhases; pods = $taskPodsChecked; logStreams = $taskPodsChecked})
    Write-Host "PASS $taskName : $taskExpected, $taskPodsChecked real Pods and SSE log streams"
}

try {
    $taskNodes = Invoke-KubectlJson -Arguments @('get', 'nodes', '-o', 'json')
    Assert-Check ($taskNodes.items.Count -gt 0) 'Cluster has no nodes'
    Test-Workflow $false
    Test-Workflow $true
    [PSCustomObject]@{context = $KubeContext; api = $ApiBaseUrl; run = $taskPrefix; results = $taskResults.ToArray(); resourcesRetained = [bool]$KeepResources}
} finally {
    if (-not $KeepResources) {
        foreach ($taskName in $taskCreatedWorkflows) {
            Write-Host "Deleting test workflow $taskName"
            Invoke-RestMethod -Method Delete -Uri "$ApiBaseUrl/api/v1/workflows/$taskName" -TimeoutSec 20 | Out-Null
            $taskDeadline = (Get-Date).AddSeconds(90)
            do {
                $taskRemaining = Invoke-KubectlJson -Arguments @('get', 'workflow', $taskName, '--ignore-not-found', '-o', 'json')
                if (-not $taskRemaining) { break }
                Start-Sleep -Seconds 1
            } while ((Get-Date) -lt $taskDeadline)
            Assert-Check (-not $taskRemaining) "Workflow cleanup timed out: $taskName"
            $taskChildren = Invoke-KubectlJson -Arguments @('get', 'tasks', '-l', "kubetask.io/workflow=$taskName", '-o', 'json')
            Assert-Check ($taskChildren.items.Count -eq 0) "Child Task cleanup incomplete: $taskName"
        }
        foreach ($taskJobName in $taskCreatedJobs) {
            $taskDeadline = (Get-Date).AddSeconds(90)
            do {
                $taskJob = Invoke-KubectlJson -Arguments @('get', 'job', $taskJobName, '-n', 'default', '--ignore-not-found', '-o', 'json')
                $taskPods = Invoke-KubectlJson -Arguments @('get', 'pods', '-n', 'default', '-l', "job-name=$taskJobName", '-o', 'json')
                if (-not $taskJob -and $taskPods.items.Count -eq 0) { break }
                Start-Sleep -Seconds 1
            } while ((Get-Date) -lt $taskDeadline)
            Assert-Check (-not $taskJob -and $taskPods.items.Count -eq 0) "Job/Pod cleanup incomplete: $taskJobName"
        }
    }
}
