<script>
  import { onMount } from 'svelte'
  import { serverProcIds, triggerPublicIP, publicIPClosedSig } from './serverStore.js'
  import { reindexStore } from './storeUtils.js'
  import {
    SaveConfig, StartService, StopService,
    StopAllServers, ScanPorts, DetectPublicIP,
    ExportService, GetServerStates
  } from '../../wailsjs/go/main/App.js'
  import LogPanel from './LogPanel.svelte'

  export let config
  export let states
  export let logs
  export let onConfigChange
  export let onStatesChange
  export let onClearLogs
  export let onAddLog
  export let onToast

  onMount(async () => {
    try {
      const currentStates = await GetServerStates()
      const newProcIds = { ...$serverProcIds }
      const usedProcIds = new Set()
      for (const [idx, pid] of Object.entries(newProcIds)) {
        if (pid == null) { delete newProcIds[idx]; continue }
        const s = currentStates.find(st => st.index === pid)
        if (s && s.status === 'running') {
          usedProcIds.add(pid)
        } else {
          delete newProcIds[idx]
        }
      }
      for (let i = 0; i < config.services.length; i++) {
        if (newProcIds[i] != null) continue
        const match = currentStates.find(s =>
          s.name === config.services[i].name && s.status === 'running' && !usedProcIds.has(s.index)
        )
        if (match) {
          usedProcIds.add(match.index)
          newProcIds[i] = match.index
        }
      }
      $serverProcIds = newProcIds
    } catch (e) {
      // ignore
    }
    if (onStatesChange) await onStatesChange()
  })

  let showAutoDialog = false
  let autoScanName = '游戏服务'
  let autoScanPattern = 'cs'
  let dialogLogs = []
  let loadingIndex = -1
  let processingAll = false
  let scanning = false
  let scanResults = []
  let selectedPorts = new Set()

  let showStunDialog = false
  let stunServer = 'stun.l.google.com:19302'
  let stunCustomServer = ''
  let stunUseCustom = false
  let stunDetecting = false
  let stunResult = null
  let stunLog = []
  const stunPresets = [
    'stun.l.google.com:19302',
    'stun.miwifi.com:3478',
    'stun.voipstunt.com:3478',
  ]

  let showPublicIPDialog = false
  let pubIPManual = ''
  let pubIPHasPublic = false
  let pubStunPreset = stunPresets[0]
  let pubStunCustom = ''
  let pubStunUseCustom = false

  function getStunServer() {
    return stunUseCustom ? stunCustomServer : stunServer
  }

  function getPubStunServer() {
    return pubStunUseCustom ? pubStunCustom : pubStunPreset
  }

  function procIdFor(index) {
    return $serverProcIds[index]
  }

  function getState(index) {
    const pid = procIdFor(index)
    if (pid == null) return 'stopped'
    const s = states.find(s => s.index === pid)
    return s ? s.status : 'stopped'
  }

  function statusLabel(status) {
    return { running: '运行中', starting: '启动中', stopped: '已停止', error: '错误' }[status] || status
  }

  function isRunning(index) {
    return getState(index) === 'running'
  }

  $: anyRunning = Object.values($serverProcIds).some(pid => pid != null && states.some(s => s.index === pid && s.status === 'running'))

  async function toggleService(index) {
    loadingIndex = index
    const srv = config.services[index]
    const state = getState(index)
    try {
      if (state === 'running') {
        const pid = procIdFor(index)
        if (onAddLog) onAddLog({ cmd: '服务端', line: `正在停止服务 ${srv.name}...` })
        try {
          await StopService(pid)
        } catch (e) {
          if (onAddLog) onAddLog({ cmd: '服务端', line: `${srv.name}: 停止错误: ${e}` })
        }
        const { [index]: _, ...restProcIds } = $serverProcIds
        $serverProcIds = restProcIds
        if (onAddLog) onAddLog({ cmd: '服务端', line: `服务 ${srv.name} 已停止` })
      } else {
        if (srv.transport === 'direct' && (!config.general.has_public_ip || !config.general.public_ip)) {
          loadingIndex = -1
          openPublicIPDialog()
          return
        }
        if (onAddLog) onAddLog({ cmd: '服务端', line: `正在启动服务 ${srv.name}...` })
        const result = await StartService(index)
        $serverProcIds = { ...$serverProcIds, [index]: result.procId }
        if (onAddLog) onAddLog({ cmd: '服务端', line: `服务 ${srv.name} 已启动` })
      }
    } catch (e) {
        if (onAddLog) onAddLog({ cmd: '服务端', line: `错误: ${e}` })
    }
    loadingIndex = -1
    if (onStatesChange) await onStatesChange()
  }

  async function handleStartAll() {
    processingAll = true
    if (onAddLog) onAddLog({ cmd: '服务端', line: '正在启动所有服务...' })
    for (const srv of config.services) {
      if (!srv.enabled) continue
      if (srv.transport === 'direct' && (!config.general.has_public_ip || !config.general.public_ip)) {
        processingAll = false
        openPublicIPDialog()
        return
      }
      if (srv.connect_method === 'raw' && srv.transport === 'direct') {
        if (onAddLog) onAddLog({ cmd: '服务端', line: `${srv.name}: 直连地址 ${config.general.public_ip}:${srv.local_port}` })
      }
    }
    let started = 0
    for (let i = 0; i < config.services.length; i++) {
      const srv = config.services[i]
      if (!srv.enabled) continue
      if (srv.connect_method === 'raw' && srv.transport === 'direct') continue
      if (onAddLog) onAddLog({ cmd: '服务端', line: `${srv.name}: 正在启动...` })
      try {
        const result = await StartService(i)
        $serverProcIds = { ...$serverProcIds, [i]: result.procId }
        started++
      } catch (e) {
        if (onAddLog) onAddLog({ cmd: '服务端', line: `${srv.name}: 错误: ${e}` })
      }
    }
    if (onAddLog) onAddLog({ cmd: '服务端', line: `全部启动完成，已启动 ${started} 个服务` })
    processingAll = false
    if (onStatesChange) await onStatesChange()
  }

  async function handleStopAll() {
    processingAll = true
    await StopAllServers()
    processingAll = false
    if (onStatesChange) await onStatesChange()
  }

  async function addService() {
    config.services = [...config.services, {
      name: '新服务',
      protocol: 'tcp',
      enabled: true,
      local_port: 25565,
      transport: 'direct',
      serv_addr: '',
      frps_port: 7000,
      frps_token: '',
      remote_port: 25565,
      connect_method: 'raw',
      wstunnel_local_port: 0,
      wstunnel_port: 0,
    }]
    await SaveConfig(config)
    await onConfigChange()
  }

  async function removeService(index) {
    if (isRunning(index)) return
    config.services = config.services.filter((_, i) => i !== index)
    $serverProcIds = reindexStore($serverProcIds, index)
    await SaveConfig(config)
    await onConfigChange()
  }

  async function updateService(index, field, value) {
    config.services = config.services.map((s, idx) =>
      idx === index ? { ...s, [field]: value } : s
    )
    await SaveConfig(config)
    await onConfigChange()
  }

  async function updateGeneral(field, value) {
    config.general[field] = value
    await SaveConfig(config)
    await onConfigChange()
  }

  async function exportService(index) {
    try {
      const code = await ExportService(index)
      await navigator.clipboard.writeText(code)
      if (onAddLog) onAddLog({ cmd: '导出连接码', line: `连接码: ${code.substring(0, 40)}...` })
      if (onToast) onToast('连接码已复制到剪贴板')
    } catch (e) {
      if (onAddLog) onAddLog({ cmd: '导出连接码', line: `导出失败: ${e}` })
      if (onToast) onToast('导出失败')
    }
  }

  function openAutoDialog() {
    dialogLogs = []
    scanResults = []
    selectedPorts = new Set()
    showAutoDialog = true
  }

  function openStunDialog() {
    stunResult = null
    stunLog = []
    showStunDialog = true
  }

  function closeStunDialog() {
    showStunDialog = false
  }

  function openPublicIPDialog() {
    pubIPManual = config.general.public_ip || ''
    pubIPHasPublic = config.general.has_public_ip
    pubStunPreset = stunPresets[0]
    pubStunCustom = ''
    pubStunUseCustom = false
    showPublicIPDialog = true
    if (config.general.auto_detect_ip && !config.general.public_ip) {
      setTimeout(() => startPubIPStunDetect(), 300)
    }
  }

  function closePublicIPDialog() {
    showPublicIPDialog = false
    publicIPClosedSig.update(n => n + 1)
  }

  async function autoSavePubIPConfig() {
    config.general.has_public_ip = pubIPHasPublic
    config.general.public_ip = pubIPManual
    config.general.auto_detect_ip = false
    await SaveConfig(config)
    await onConfigChange()
  }

  let showSvcSettingsDialog = false
  let settingsSvcIdx = -1
  let settingsSvc = {}

  function openSvcSettings(index) {
    settingsSvcIdx = index
    settingsSvc = { ...config.services[index] }
    if (!config.general.has_public_ip && settingsSvc.transport === 'direct') {
      settingsSvc.transport = 'frp'
    }
    showSvcSettingsDialog = true
  }

  function closeSvcSettings() {
    showSvcSettingsDialog = false
    settingsSvcIdx = -1
    settingsSvc = {}
  }

  async function autoSaveSvcSettings() {
    if (settingsSvcIdx < 0) return
    config.services[settingsSvcIdx] = { ...settingsSvc }
    await SaveConfig(config)
    await onConfigChange()
  }

  function stunLogMsg(msg) {
    stunLog = [...stunLog, msg]
  }

  async function startStunDetect() {
    stunDetecting = true
    stunResult = null
    stunLog = []
    stunLogMsg(`STUN 服务器: ${getStunServer()}`)
    stunLogMsg('正在检测...')
    try {
      const res = await DetectPublicIP(getStunServer())
      stunResult = res
      if (res.public_ip) {
        stunLogMsg(`公网 IP: ${res.public_ip}`)
        config.general.has_public_ip = res.has_public_ip
        config.general.public_ip = res.public_ip
        config.general.auto_detect_ip = true
        await SaveConfig(config)
        await onConfigChange()
        if (res.has_public_ip) {
          stunLogMsg('✅ 已自动保存配置，该 IP 属于本机')
        } else {
          stunLogMsg('⚠️ 已自动保存，该 IP 不在本机网络接口上，可能是 NAT 映射地址')
        }
      } else {
        stunLogMsg('❌ 未检测到公网 IP')
        stunLogMsg('建议选择"无公网 IP"，使用 frp 中继')
      }
    } catch (e) {
      stunLogMsg(`❌ 检测失败: ${e}`)
      stunLogMsg('建议选择"无公网 IP"，使用 frp 中继')
    }
    stunDetecting = false
  }

  async function applyStunResult() {
    if (!stunResult) return
    config.general.has_public_ip = stunResult.has_public_ip
    config.general.public_ip = stunResult.public_ip || ''
    config.general.auto_detect_ip = true
    await SaveConfig(config)
    await onConfigChange()
    showStunDialog = false
  }

  async function startPubIPStunDetect() {
    stunDetecting = true
    stunResult = null
    stunLog = []
    stunLogMsg(`STUN 服务器: ${getPubStunServer()}`)
    stunLogMsg('正在检测公网 IP...')
    try {
      const res = await DetectPublicIP(getPubStunServer())
      stunResult = res
      if (res.public_ip) {
        stunLogMsg(`公网 IP: ${res.public_ip}`)
        pubIPManual = res.public_ip
        pubIPHasPublic = res.has_public_ip
        await autoSavePubIPConfig()
        if (res.has_public_ip) {
          stunLogMsg('✅ 检测成功，已自动配置')
        } else {
          stunLogMsg('⚠️ 该 IP 不在本机网络接口上，请确认后手动设置')
        }
      } else {
        stunLogMsg('❌ 未检测到公网 IP')
      }
    } catch (e) {
      stunLogMsg(`❌ 检测失败: ${e}`)
    }
    stunDetecting = false
  }

  function closeAutoDialog() {
    showAutoDialog = false
  }

  function log(msg) {
    dialogLogs = [...dialogLogs, msg]
  }

  function togglePort(port) {
    const next = new Set(selectedPorts)
    if (next.has(port)) {
      next.delete(port)
    } else {
      next.add(port)
    }
    selectedPorts = next
  }

  function selectAllMatching() {
    const next = new Set()
    for (const p of scanResults) {
      if (p.process_name.toLowerCase().includes(autoScanPattern.toLowerCase())) {
        next.add(p.local_port)
      }
    }
    selectedPorts = next
  }

  async function startScan() {
    scanning = true
    dialogLogs = []
    scanResults = []
    selectedPorts = new Set()
    const pattern = `.*${autoScanPattern}.*`
    log(`搜索进程: ${autoScanPattern} (模式: .*${autoScanPattern}.*)`)
    log('正在扫描本地监听端口...')
    try {
      const ports = await ScanPorts()
      if (!ports || ports.length === 0) {
        log('未发现任何监听端口')
        scanning = false
        return
      }
      scanResults = ports
      for (const p of ports) {
        log(`[${p.process_name}] 端口 ${p.local_port}  PID ${p.process_pid}`)
      }
      selectAllMatching()
      log(`扫描完成，共 ${ports.length} 个端口，匹配 ${selectedPorts.size} 个`)
    } catch (e) {
      log(`出错: ${e}`)
    }
    scanning = false
  }

  async function confirmAdd() {
    if (selectedPorts.size === 0) {
      log('未选择任何端口')
      return
    }
    let added = 0
    for (const p of scanResults) {
      if (!selectedPorts.has(p.local_port)) continue
      config.services = [...config.services, {
        name: autoScanName,
        protocol: 'tcp',
        enabled: true,
        local_port: p.local_port,
        transport: config.general.has_public_ip ? 'direct' : 'frp',
        serv_addr: '',
        frps_port: 7000,
        frps_token: '',
        remote_port: p.local_port,
        connect_method: 'wstunnel',
        wstunnel_local_port: 0,
        wstunnel_port: 0,
      }]
      added++
    }
    if (added > 0) {
      await SaveConfig(config)
      await onConfigChange()
      log(`已添加 ${added} 个服务`)
      if (onAddLog) onAddLog({ cmd: '自动发现', line: `已添加 ${added} 个服务` })
    } else {
      log('所选端口均已存在')
    }
    showAutoDialog = false
  }

  let statusText = '已停止'
  let statusColor = 'var(--text-muted)'
  $: {
    if (anyRunning) {
      statusText = '运行中'
      statusColor = 'var(--green)'
    } else {
      statusText = '已停止'
      statusColor = 'var(--text-muted)'
    }
  }

  $: if ($triggerPublicIP > 0) {
    openPublicIPDialog()
    triggerPublicIP.set(0)
  }
</script>

<div class="server-tab">
  <div class="section">
    <div class="control-bar">
      <div class="btn-group">
        <button class="btn-primary" on:click={handleStartAll} disabled={processingAll || config.services.length === 0 || !config.services.some(s => s.enabled)}>▶ 全部启动</button>
        <button class="btn-danger" on:click={handleStopAll} disabled={processingAll}>⏹ 全部停止</button>
      </div>
      <div class="control-bar-status">
        <span class="status-dot" class:running={anyRunning}></span>
        <span class="status-text" style="color: {statusColor}">{statusText}</span>
      </div>
    </div>
  </div>

  <div class="section">
    <div class="section-title-row">
      <div class="section-title">游戏端口列表</div>
      <label class="radio-group">
        <span class="label-text">公网 IP：</span>
        <button
          class="radio-btn"
          class:active={config.general.has_public_ip}
          on:click={() => updateGeneral('has_public_ip', true)}
        >有公网 IP</button>
        <button
          class="radio-btn"
          class:active={!config.general.has_public_ip}
          on:click={() => updateGeneral('has_public_ip', false)}
        >无公网 IP</button>
        <button class="btn-ghost btn-sm" on:click={openPublicIPDialog}>公网配置</button>
      </label>
    </div>
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th class="col-name">游戏名</th>
            <th class="col-port">端口</th>
            <th class="col-status">状态</th>
            <th class="col-type">类型</th>
            <th class="col-enabled">启用</th>
            <th class="col-action">操作</th>
          </tr>
        </thead>
        <tbody>
          {#each config.services as srv, i}
            {@const _procId = $serverProcIds[i]}
            {@const _state = _procId != null ? (states.find(s => s.index === _procId)?.status) || 'stopped' : 'stopped'}
            {@const _running = _state === 'running'}
            <tr>
              <td class="col-name-cell">
                <span class="srv-name">{srv.name}</span>
              </td>
              <td class="col-port-cell">
                <span class="srv-port">{srv.local_port}</span>
              </td>
              <td class="col-status-cell">
                {#if srv.connect_method === 'raw' && srv.transport === 'direct'}
                  <span class="text-muted">无需启动</span>
                {:else}
                   <span class="status-dot" class:running={_running}></span>
                  <span class="status-label">{statusLabel(_state)}</span>
                {/if}
              </td>
              <td class="col-type-cell">
                <span class="srv-type">{srv.transport === 'wstunnel' ? 'wstunnel' : srv.connect_method === 'wstunnel' && srv.transport === 'frp' ? 'ws+frp' : srv.connect_method === 'wstunnel' ? 'ws' : srv.transport === 'frp' ? 'frp' : '直连'}</span>
              </td>
              <td class="col-enabled-cell">
                <input type="checkbox" checked={srv.enabled} on:change={(e) => updateService(i, 'enabled', e.target.checked)} disabled={_running} />
              </td>
              <td>
                <div class="btn-group">
                  <button
                    class="btn-sm btn-toggle"
                    class:btn-primary={!_running}
                    class:btn-danger={_running}
                    on:click={() => toggleService(i)}
                    disabled={loadingIndex >= 0 || !srv.enabled || srv.connect_method === 'raw' && srv.transport === 'direct' || srv.transport === 'direct' && !config.general.has_public_ip}
                  >
                    {loadingIndex === i ? '处理中...' : _running ? '停止' : '启动'}
                  </button>
                  <button
                    class="btn-action"
                    on:click={() => openSvcSettings(i)}
                  >⚙️设置</button>
                  <button
                    class="btn-action"
                    on:click={() => exportService(i)}
                  >📤导出</button>
                  <button
                    class="btn-ghost btn-sm"
                    on:click={() => removeService(i)}
                    disabled={_running}
                  >✕</button>
                </div>
              </td>
            </tr>
          {:else}
            <tr>
              <td colspan="6" class="empty-row">暂无服务，点击"🔍 自动发现"或"+ 添加服务"添加</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <div class="table-actions">
      <button class="btn-primary btn-sm btn-import-auto" on:click={openAutoDialog}>🔍 自动发现</button>
      <button class="btn-ghost btn-sm" on:click={addService}>+ 添加服务</button>
    </div>
  </div>

  <LogPanel {logs} {onClearLogs} />
</div>

{#if showAutoDialog}
  <div class="modal-overlay" on:mousedown|self={closeAutoDialog}>
    <div class="modal-dialog" on:click|stopPropagation>
      <div class="modal-header">
        <span class="modal-title">自动发现端口</span>
        <button class="btn-ghost btn-sm" on:click={closeAutoDialog}>✕</button>
      </div>
      <div class="modal-body">
        <div class="modal-form-row">
          <span class="form-label">服务名称</span>
          <input bind:value={autoScanName} placeholder="游戏服务" disabled={scanning} />
        </div>
        <div class="modal-form-row">
          <span class="form-label">搜索进程</span>
          <input bind:value={autoScanPattern} placeholder="cs" disabled={scanning} />
        </div>
        <div class="modal-scan-bar">
          <button class="btn-primary btn-sm" on:click={startScan} disabled={scanning}>
            {scanning ? '扫描中...' : '🔍 开始扫描'}
          </button>
          {#if scanResults.length > 0}
            <button class="btn-ghost btn-sm" on:click={selectAllMatching} disabled={scanning}>
              全选匹配
            </button>
            <span class="match-count">
              已选 {selectedPorts.size} / {scanResults.length}
            </span>
          {/if}
        </div>
        {#if scanResults.length > 0}
          <div class="modal-result-list">
            {#each scanResults as p}
              <label class="result-item" class:selected={selectedPorts.has(p.local_port)}>
                <input type="checkbox" checked={selectedPorts.has(p.local_port)} on:change={() => togglePort(p.local_port)} />
                <span class="result-port">{p.local_port}</span>
                <span class="result-name">{p.process_name}</span>
                <span class="result-pid">PID {p.process_pid}</span>
              </label>
            {/each}
          </div>
        {/if}
        <div class="modal-log-panel">
          {#each dialogLogs as msg}
            <div class="log-line">{msg}</div>
          {:else}
            <div class="log-empty">点击"开始扫描"查看结果</div>
          {/each}
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn-ghost" on:click={closeAutoDialog}>取消</button>
        <button class="btn-primary" on:click={confirmAdd} disabled={scanning || selectedPorts.size === 0}>
          确认添加
        </button>
      </div>
    </div>
  </div>
{/if}

{#if showStunDialog}
  <div class="modal-overlay" on:mousedown|self={closeStunDialog}>
    <div class="modal-dialog stun-dialog" on:click|stopPropagation>
      <div class="modal-header">
        <span class="modal-title">公网 IP 检测</span>
        <button class="btn-ghost btn-sm" on:click={closeStunDialog}>✕</button>
      </div>
      <div class="modal-body">
        <div class="modal-form-row">
          <span class="form-label">STUN 服务器</span>
          <select bind:value={stunServer} on:change={() => stunUseCustom = false}>
            {#each stunPresets as s}
              <option value={s}>{s}</option>
            {/each}
            <option value="">自定义</option>
          </select>
        </div>
        {#if stunServer === ''}
          <div class="modal-form-row">
            <span class="form-label">自定义地址</span>
            <input bind:value={stunCustomServer} placeholder="stun.example.com:3478" on:input={() => stunUseCustom = true} />
          </div>
        {/if}
        {#if stunResult}
          <div class="stun-result-box">
            <div class="result-row">
              <span class="result-label">公网 IP</span>
              <span class="result-value">{stunResult.public_ip || '无'}</span>
            </div>
            <div class="result-row">
              <span class="result-label">属于本机</span>
              <span class="result-value">{stunResult.has_public_ip ? '✅ 是' : '❌ 否'}</span>
            </div>
            <div class="result-row">
              <span class="result-label">建议</span>
              <span class="result-value">{stunResult.has_public_ip ? '选择"有公网 IP"，可使用直连' : '选择"无公网 IP"，建议使用 frp 中继'}</span>
            </div>
          </div>
        {/if}
        <div class="modal-log-panel">
          {#each stunLog as msg}
            <div class="log-line">{msg}</div>
          {:else}
            <div class="log-empty">点击"开始检测"查看结果</div>
          {/each}
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn-ghost" on:click={closeStunDialog}>关闭</button>
        <button class="btn-primary" on:click={startStunDetect} disabled={stunDetecting}>
          {stunDetecting ? '检测中...' : '🛰 开始检测'}
        </button>
      </div>
    </div>
  </div>
{/if}

{#if showPublicIPDialog}
  <div class="modal-overlay" on:mousedown|self={closePublicIPDialog}>
    <div class="modal-dialog stun-dialog" on:click|stopPropagation>
      <div class="modal-header">
        <span class="modal-title">公网 IP 配置</span>
        <button class="btn-ghost btn-sm" on:click={closePublicIPDialog}>✕</button>
      </div>
      <div class="modal-body">
        <div class="modal-form-row">
          <span class="form-label">公网 IP</span>
          <input bind:value={pubIPManual} placeholder="1.2.3.4" style="flex:1" on:input={autoSavePubIPConfig} />
        </div>
        <div class="modal-scan-bar">
          <select bind:value={pubStunPreset} on:change={() => pubStunUseCustom = false} style="flex:1; min-width:0">
            {#each stunPresets as s}
              <option value={s}>{s}</option>
            {/each}
            <option value="">自定义</option>
          </select>
          {#if pubStunPreset === ''}
            <input bind:value={pubStunCustom} placeholder="stun.example.com:3478" style="width:200px; flex-shrink:0" on:input={() => pubStunUseCustom = true} />
          {/if}
          <button class="btn-primary btn-sm detect-fixed-btn" on:click={startPubIPStunDetect} disabled={stunDetecting}>
            {stunDetecting ? '检测中...' : '🛰 检测'}
          </button>
        </div>
        {#if stunResult}
          <div class="stun-result-box">
            <div class="result-row">
              <span class="result-label">检测结果</span>
              <span class="result-value">{stunResult.public_ip || '无'}</span>
            </div>
            <div class="result-row">
              <span class="result-label">属于本机</span>
              <span class="result-value">{stunResult.has_public_ip ? '✅ 是' : '❌ 否'}</span>
            </div>
          </div>
        {/if}
        <div class="modal-log-panel">
          {#each stunLog as msg}
            <div class="log-line">{msg}</div>
          {:else}
            <div class="log-empty">可手动输入公网 IP 或点击检测</div>
          {/each}
        </div>
      </div>
      <div class="modal-footer">
        <button class="btn-primary" on:click={closePublicIPDialog}>关闭</button>
      </div>
    </div>
  </div>
{/if}

{#if showSvcSettingsDialog}
  <div class="modal-overlay" on:mousedown|self={closeSvcSettings}>
    <div class="modal-dialog" on:click|stopPropagation>
      <div class="modal-header">
        <span class="modal-title">服务设置 — {settingsSvc.name}</span>
        <button class="btn-ghost btn-sm" on:click={closeSvcSettings}>✕</button>
      </div>
      <div class="modal-body">
        <div class="modal-form-row">
          <span class="form-label">服务名称</span>
          <input bind:value={settingsSvc.name} style="flex:1" on:input={autoSaveSvcSettings} />
        </div>
        <div class="modal-form-row">
          <span class="form-label">协议</span>
          <select bind:value={settingsSvc.protocol} style="flex:1" on:change={autoSaveSvcSettings}>
            <option value="tcp">TCP</option>
            <option value="udp">UDP</option>
          </select>
        </div>
        <div class="modal-form-row">
          <span class="form-label">本地端口</span>
          <input type="number" bind:value={settingsSvc.local_port} style="flex:1" on:input={autoSaveSvcSettings} />
        </div>
        <div class="modal-form-row">
          <span class="form-label">传输路径</span>
          <select bind:value={settingsSvc.transport} style="flex:1" on:change={autoSaveSvcSettings}>
            {#if config.general.has_public_ip}
              <option value="direct">直连</option>
            {/if}
            <option value="frp">frp 中继</option>
            <option value="wstunnel">wstunnel</option>
          </select>
        </div>
        {#if settingsSvc.transport === 'frp'}
          <div class="modal-form-row">
            <span class="form-label">目标服务器</span>
            <input bind:value={settingsSvc.serv_addr} placeholder="relay.example.com" style="flex:1" on:input={autoSaveSvcSettings} />
          </div>
          <div class="modal-form-row">
            <span class="form-label">Frp端口</span>
            <input type="number" bind:value={settingsSvc.frps_port} style="flex:1" on:input={autoSaveSvcSettings} />
          </div>
          <div class="modal-form-row">
            <span class="form-label">Auth Token</span>
            <input type="password" bind:value={settingsSvc.frps_token} placeholder="留空则不鉴权" style="flex:1" on:input={autoSaveSvcSettings} />
          </div>
          <div class="modal-form-row">
            <span class="form-label">远程端口</span>
            <input type="number" bind:value={settingsSvc.remote_port} style="flex:1" on:input={autoSaveSvcSettings} />
          </div>
        {:else if settingsSvc.transport === 'wstunnel'}
          <div class="modal-form-row">
            <span class="form-label">目标服务器</span>
            <input bind:value={settingsSvc.serv_addr} placeholder="relay.example.com" style="flex:1" on:input={autoSaveSvcSettings} />
          </div>
          <div class="modal-form-row">
            <span class="form-label">远程端口</span>
            <input type="number" bind:value={settingsSvc.remote_port} style="flex:1" on:input={autoSaveSvcSettings} />
          </div>
          <div class="modal-form-row">
            <span class="form-label">WS端口</span>
            <input type="number" bind:value={settingsSvc.wstunnel_port} placeholder="443" style="flex:1" on:input={autoSaveSvcSettings} />
          </div>
        {/if}
        <div class="modal-form-row">
          <span class="form-label">玩家连接方式</span>
          <select bind:value={settingsSvc.connect_method} style="flex:1" on:change={autoSaveSvcSettings}>
            <option value="raw">直连</option>
            <option value="wstunnel">wstunnel</option>
          </select>
        </div>
        {#if settingsSvc.connect_method === 'wstunnel' && settingsSvc.transport !== 'wstunnel'}
          <div class="modal-form-row">
            <span class="form-label">WS端口</span>
            <input type="number" bind:value={settingsSvc.wstunnel_port} style="flex:1" on:input={autoSaveSvcSettings} />
          </div>
        {/if}
      </div>
      <div class="modal-footer">
        <button class="btn-primary" on:click={closeSvcSettings}>关闭</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .server-tab {
    height: 100%;
    display: flex;
    flex-direction: column;
    padding: 12px 20px;
    gap: 12px;
    overflow: hidden;
  }

  .radio-group {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .label-text {
    color: var(--text-secondary);
    font-size: 13px;
  }

  .radio-btn {
    background: var(--bg-card);
    color: var(--text-secondary);
    border: 1px solid var(--border);
    padding: 2px 10px;
    font-size: 13px;
  }

  .radio-btn.active {
    background: var(--accent-dim);
    color: var(--accent);
    border-color: var(--accent);
  }

  [data-theme="light"] .radio-btn.active {
    background: #000000;
    color: #ffffff;
    border: none;
  }

  .section {
    flex-shrink: 0;
  }

  .section-title-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 28px;
    margin-bottom: 8px;
  }

  .section-title {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.5px;
  }

  .table-wrap {
    overflow-x: auto;
    overflow-y: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    height: 200px;
  }

  table {
    width: 100%;
    table-layout: fixed;
    border-collapse: collapse;
    font-size: 13px;
  }

  tbody tr {
    height: 50px;
  }

  th {
    text-align: center;
    padding: 8px 10px;
    background: var(--bg-secondary);
    color: var(--text-secondary);
    font-weight: 500;
    white-space: nowrap;
    border-bottom: 1px solid var(--border);
  }

  td {
    padding: 6px 10px;
    border-bottom: 1px solid var(--border);
    vertical-align: middle;
  }

  tr:last-child td {
    border-bottom: none;
  }

  .col-name { min-width: 140px; }
  .col-type { width: 80px; }
  .col-enabled { width: 50px; }
  .col-port { width: 80px; }
  .col-status { width: 100px; }
  .col-action { width: 240px; }

  .col-name-cell {
    text-align: left;
    vertical-align: middle;
  }
  .col-enabled-cell {
    text-align: center;
    vertical-align: middle;
  }
  .col-enabled-cell input {
    cursor: pointer;
  }

  .col-type-cell {
    text-align: center;
    vertical-align: middle;
  }
  .col-port-cell {
    text-align: center;
    vertical-align: middle;
  }
  .col-status-cell {
    text-align: center;
    vertical-align: middle;
  }

  .srv-name { font-weight: 500; }

  .srv-type {
    display: inline-block;
    margin-left: 6px;
    padding: 1px 6px;
    border-radius: 3px;
    background: var(--accent-dim);
    color: var(--accent);
    font-size: 11px;
    font-weight: 500;
    vertical-align: middle;
  }

  .srv-port {
    font-family: 'Cascadia Code', 'Fira Code', 'Consolas', monospace;
    font-size: 13px;
    font-weight: 500;
  }

  .status-dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--text-muted);
    margin-right: 6px;
    vertical-align: middle;
  }

  .status-dot.running {
    background: var(--green);
    box-shadow: 0 0 6px var(--green);
  }

  .status-label {
    font-size: 12px;
    color: var(--text-secondary);
    vertical-align: middle;
  }

  .empty-row {
    text-align: center;
    color: var(--text-muted);
    padding: 30px 10px;
  }

  .table-actions {
    display: flex;
    gap: 8px;
    margin-top: 8px;
    align-items: center;
  }

  .btn-group {
    display: flex;
    gap: 6px;
    align-items: center;
  }

  .btn-import-auto {
    width: 120px;
  }

  .settings-grid {
    display: flex;
    flex-direction: column;
    gap: 10px;
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 14px;
  }

  .setting-row {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .setting-label {
    color: var(--text-secondary);
    font-size: 13px;
    white-space: nowrap;
  }

  .token-input {
    width: 120px;
  }

  .control-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 10px 14px;
  }

  .control-bar-status {
    display: flex;
    align-items: center;
  }

  .status-text {
    font-weight: 600;
    font-size: 14px;
  }

  .modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }

  .modal-dialog {
    background: var(--bg-primary);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    width: 600px;
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    box-shadow: var(--shadow);
  }

  .modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 14px 18px;
    border-bottom: 1px solid var(--border);
  }

  .modal-title {
    font-size: 15px;
    font-weight: 600;
  }

  .modal-body {
    flex: 1;
    overflow-y: auto;
    padding: 14px 18px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .modal-form-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .form-label {
    color: var(--text-secondary);
    font-size: 13px;
    white-space: nowrap;
    width: 70px;
  }

  .modal-form-row input {
    flex: 1;
    padding: 6px 10px;
    font-size: 13px;
  }

  .modal-scan-bar {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .detect-fixed-btn {
    width: 90px;
    flex-shrink: 0;
    margin-left: auto;
  }

  .match-count {
    color: var(--text-muted);
    font-size: 12px;
    margin-left: auto;
  }

  .modal-result-list {
    max-height: 200px;
    overflow-y: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }

  .result-item {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 10px;
    font-size: 13px;
    cursor: pointer;
    border-bottom: 1px solid var(--border);
    transition: background 0.1s;
  }

  .result-item:last-child {
    border-bottom: none;
  }

  .result-item:hover {
    background: var(--bg-hover);
  }

  .result-item.selected {
    background: var(--accent-dim);
  }

  .result-item input {
    cursor: pointer;
  }

  .result-port {
    font-weight: 600;
    width: 60px;
    color: var(--accent);
  }

  .result-name {
    flex: 1;
    color: var(--text-primary);
  }

  .result-pid {
    color: var(--text-muted);
    font-size: 11px;
  }

  .modal-log-panel {
    background: var(--bg-log);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 8px 12px;
    min-height: 80px;
    max-height: 150px;
    overflow-y: auto;
    font-family: 'Cascadia Code', 'Fira Code', 'Consolas', monospace;
    font-size: 12px;
    line-height: 1.6;
    user-select: text;
    -webkit-user-select: text;
  }

  .modal-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 12px 18px;
    border-top: 1px solid var(--border);
  }

  .modal-subsection {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-secondary);
    margin-top: 4px;
    margin-bottom: 2px;
  }

  .modal-body .modal-form-row input {
    padding: 6px 10px;
    font-size: 13px;
  }

  .stun-dialog {
    width: 480px;
  }

  .stun-result-box {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .result-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .result-label {
    color: var(--text-secondary);
    font-size: 13px;
    width: 70px;
    flex-shrink: 0;
  }

  .result-value {
    font-size: 13px;
    font-weight: 500;
  }

  .stun-result-box {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .result-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .result-label {
    color: var(--text-secondary);
    font-size: 13px;
    width: 70px;
    flex-shrink: 0;
  }

  .result-value {
    font-size: 13px;
    font-weight: 500;
  }
</style>
