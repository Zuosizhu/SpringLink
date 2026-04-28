<script>
  import { onMount } from 'svelte'
  import { clientProcIds, clientAddrs, triggerImport } from './clientStore.js'
  import { reindexStore } from './storeUtils.js'
  import LogPanel from './LogPanel.svelte'
  import {
    SaveConfig, GetClientStates, ExportClientService,
    StartAllClients, StartClientService, StopClientService, StopAllClients,
    PreviewService
  } from '../../wailsjs/go/main/App.js'

  export let config
  export let logs
  export let states
  export let onConfigChange
  export let onStatesChange
  export let onClearLogs
  export let onAddLog
  export let onToast

  let clientServices = []
  let showImportDialog = false
  let importCode = ''
  let importPreview = null
  let importError = ''
  let importing = false
  let previewTimer
  let showSettingsDialog = false
  let settingsService = null
  let settingsIndex = -1
  let loadingIndex = -1
  let processingAll = false

  function onImportCodeInput() {
    clearTimeout(previewTimer)
    importPreview = null
    importError = ''
    if (!importCode.trim()) return
    previewTimer = setTimeout(() => {
      previewImportCode()
    }, 500)
  }

  async function pasteFromClipboard() {
    try {
      importCode = await navigator.clipboard.readText()
      onImportCodeInput()
    } catch (e) {
      importError = '无法读取剪贴板'
    }
  }

  function loadFromConfig() {
    const globalServerAddr = config.client?.serv_addr || ''
    clientServices = (config.client?.services || []).map((s) => ({
      ...s,
      serv_addr: s.serv_addr ?? globalServerAddr,
    }))
  }

  async function saveToConfig() {
    config.client = {
      serv_addr: config.client?.serv_addr || '',
      services: clientServices.map(s => ({
        name: s.name, protocol: s.protocol, enabled: s.enabled,
        local_port: s.local_port, connect_method: s.connect_method,
        remote_port: s.remote_port,
        wstunnel_local_port: s.wstunnel_local_port || 0,
        wstunnel_port: s.wstunnel_port || 0,
        serv_addr: s.serv_addr,
      })),
    }
    await SaveConfig(config)
    if (onConfigChange) onConfigChange()
  }

  function connAddr(srv, index) {
    if ($clientAddrs[index]) return $clientAddrs[index]
    if (srv.connect_method === 'raw') return `${srv.serv_addr || '-'}:${srv.remote_port || srv.local_port}`
    return ''
  }

  function connTypeLabel(srv) {
    if (srv.connect_method === 'wstunnel') return 'ws'
    if (srv.connect_method === 'raw') return '直连'
    return ''
  }

  onMount(async () => {
    loadFromConfig()
    try {
      const currentStates = await GetClientStates()
      const newProcIds = { ...$clientProcIds }
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
      for (let i = 0; i < clientServices.length; i++) {
        if (newProcIds[i] != null) continue
        const match = currentStates.find(s =>
          s.name === clientServices[i].name && s.status === 'running' && !usedProcIds.has(s.index)
        )
        if (match) {
          usedProcIds.add(match.index)
          newProcIds[i] = match.index
        }
      }
      $clientProcIds = newProcIds
    } catch (e) {
      // ignore
    }
    if (onStatesChange) onStatesChange()
  })

  function procIdFor(index) {
    return $clientProcIds[index]
  }

  function getState(index) {
    const pid = procIdFor(index)
    if (pid == null) return 'stopped'
    const s = states.find(s => s.index === pid)
    return s ? s.status : 'stopped'
  }

  function statusLabel(status) {
    return { running: '已连接', starting: '连接中', stopped: '未连接', error: '错误' }[status] || status
  }

  function isRunning(index) {
    return getState(index) === 'running'
  }

  function isAnyRunning() {
    return clientServices.some((_, i) => isRunning(i))
  }

  async function handleImport() {
    if (onAddLog) onAddLog({ cmd: '客户端', line: '打开导入连接码对话框' })
    showImportDialog = true
    importCode = ''
    importPreview = null
    importError = ''
    clearTimeout(previewTimer)
  }

  async function previewImportCode() {
    importError = ''
    importPreview = null
    if (!importCode.trim()) {
      importError = '请输入连接码'
      return
    }
    try {
      importPreview = await PreviewService(importCode.trim())
    } catch (e) {
      importError = `解码失败: ${e}`
    }
  }

  async function confirmImport() {
    if (!importPreview) return
    importing = true
    try {
      if (onAddLog) onAddLog({ cmd: '连接码', line: `正在导入: ${importPreview.n}` })
      const host = importPreview.rh || importPreview.sa || ''
      clientServices = [...clientServices, {
        name: importPreview.n,
        protocol: importPreview.p,
        enabled: true,
        local_port: importPreview.lp,
        connect_method: importPreview.c,
        remote_port: importPreview.rp,
        wstunnel_local_port: 0,
        wstunnel_port: importPreview.wp || 0,
        serv_addr: host,
      }]
      await saveToConfig()
      showImportDialog = false
      if (onToast) onToast('连接码导入成功')
    } catch (e) {
      importError = `导入失败: ${e}`
    }
    importing = false
  }

  function closeImportDialog() {
    showImportDialog = false
  }

  async function exportService(index) {
    const srv = clientServices[index]
    if (!srv) return
    try {
      const code = await ExportClientService({
        name: srv.name,
        protocol: srv.protocol,
        enabled: true,
        local_port: srv.local_port,
        remote_port: srv.remote_port,
        connect_method: srv.connect_method,
        wstunnel_local_port: srv.wstunnel_local_port || 0,
        wstunnel_port: srv.wstunnel_port || 0,
        serv_addr: srv.serv_addr,
      })
      await navigator.clipboard.writeText(code)
      if (onToast) onToast('连接码已复制到剪贴板')
      if (onAddLog) onAddLog({ cmd: '导出连接码', line: `连接码: ${code.substring(0, 40)}...` })
    } catch (e) {
      if (onAddLog) onAddLog({ cmd: '导出连接码', line: `错误: ${e}` })
    }
  }

  function openSettings(index) {
    settingsIndex = index
    settingsService = JSON.parse(JSON.stringify(clientServices[index]))
    showSettingsDialog = true
  }

  function closeSettings() {
    showSettingsDialog = false
    settingsService = null
    settingsIndex = -1
  }

  async function autoSaveSettings() {
    if (settingsIndex < 0 || !settingsService) return
    clientServices[settingsIndex] = { ...settingsService }
    await saveToConfig()
  }

  async function toggleEnabled(index) {
    clientServices = clientServices.map((s, i) => i === index ? { ...s, enabled: !s.enabled } : s)
    await saveToConfig()
  }

  async function addClientService() {
    const srv = {
      name: '新服务',
      protocol: 'tcp',
      enabled: true,
      local_port: 25565,
      connect_method: 'raw',
      remote_port: 25565,
      wstunnel_local_port: 0,
      wstunnel_port: 0,
      serv_addr: config.client?.serv_addr || '',
    }
    clientServices = [...clientServices, srv]
    if (onAddLog) onAddLog({ cmd: '客户端', line: `已添加: ${srv.name}` })
    await saveToConfig()
  }

  async function removeService(index) {
    const srv = clientServices[index]
    if (srv && isRunning(index)) return
    if (onAddLog && srv) onAddLog({ cmd: '客户端', line: `已移除: ${srv.name}` })
    clientServices = clientServices.filter((_, i) => i !== index)
    $clientProcIds = reindexStore($clientProcIds, index)
    $clientAddrs = reindexStore($clientAddrs, index)
    await saveToConfig()
  }

  async function handleConnectAll() {
    if (clientServices.length === 0) return
    processingAll = true
    if (onAddLog) onAddLog({ cmd: '客户端', line: '正在连接所有服务...' })
    for (const srv of clientServices) {
      if (!srv.enabled) continue
      if (srv.connect_method !== 'raw' && !srv.serv_addr) {
        if (onAddLog) onAddLog({ cmd: '客户端', line: `${srv.name}: 跳过，缺少服务器地址` })
        continue
      }
    }
    let connected = 0
    for (let i = 0; i < clientServices.length; i++) {
      const srv = clientServices[i]
      if (!srv.enabled) continue
      if (srv.connect_method === 'raw') {
        if (onAddLog) onAddLog({ cmd: '客户端', line: `${srv.name}: 直连地址 ${connAddr(srv, i)}` })
        $clientAddrs = { ...$clientAddrs, [i]: connAddr(srv, i) }
      } else if (srv.serv_addr) {
        if (onAddLog) onAddLog({ cmd: '客户端', line: `${srv.name}: 正在连接...` })
        try {
          const result = await StartClientService({
            name: srv.name, protocol: srv.protocol, enabled: true,
            local_port: srv.local_port, connect_method: srv.connect_method,
            remote_port: srv.remote_port,
            wstunnel_local_port: srv.wstunnel_local_port || 0,
            wstunnel_port: srv.wstunnel_port || 0,
          }, srv.serv_addr)
          connected++
          $clientProcIds = { ...$clientProcIds, [i]: result.procId }
          if (result.localAddr) {
            $clientAddrs = { ...$clientAddrs, [i]: result.localAddr }
            if (onAddLog) onAddLog({ cmd: '客户端', line: `${srv.name}: 已连接, 本地地址 ${result.localAddr}` })
          } else if (result.connAddr) {
            $clientAddrs = { ...$clientAddrs, [i]: result.connAddr }
            if (onAddLog) onAddLog({ cmd: '客户端', line: `${srv.name}: 连接地址 ${result.connAddr}` })
          }
        } catch (e) {
          if (onAddLog) onAddLog({ cmd: '客户端', line: `${srv.name}: 错误: ${e}` })
        }
      }
    }
    if (onStatesChange) await onStatesChange()
    processingAll = false
    if (onAddLog) onAddLog({ cmd: '客户端', line: `全部连接完成，已连接 ${connected} 个服务` })
  }

  async function handleDisconnectAll() {
    processingAll = true
    if (onAddLog) onAddLog({ cmd: '客户端', line: '正在断开所有服务...' })
    await StopAllClients()
    if (onStatesChange) await onStatesChange()
    processingAll = false
    if (onAddLog) onAddLog({ cmd: '客户端', line: '已断开所有服务' })
  }

  async function toggleService(index) {
    loadingIndex = index
    const srv = clientServices[index]
    if (!srv) {
      loadingIndex = -1
      return
    }
    if (getState(index) === 'running') {
      if (onAddLog) onAddLog({ cmd: '客户端', line: `${srv.name}: 正在断开...` })
      try {
        await StopClientService(procIdFor(index))
      } catch (e) {
        if (onAddLog) onAddLog({ cmd: '客户端', line: `${srv.name}: 断开错误: ${e}` })
      }
      const { [index]: _, ...restProcIds } = $clientProcIds
      $clientProcIds = restProcIds
      const { [index]: _a, ...restAddrs } = $clientAddrs
      $clientAddrs = restAddrs
      if (onAddLog) onAddLog({ cmd: '客户端', line: `${srv.name}: 已断开` })
    } else if (srv.connect_method === 'raw') {
      if (onAddLog) onAddLog({ cmd: '客户端', line: `${srv.name}: 直连地址 ${connAddr(srv, index)}` })
      $clientAddrs = { ...$clientAddrs, [index]: connAddr(srv, index) }
    } else if (srv.serv_addr) {
      if (onAddLog) onAddLog({ cmd: '客户端', line: `${srv.name}: 正在连接...` })
      try {
        const result = await StartClientService({
          name: srv.name, protocol: srv.protocol, enabled: true,
          local_port: srv.local_port, connect_method: srv.connect_method,
          remote_port: srv.remote_port,
          wstunnel_local_port: srv.wstunnel_local_port || 0,
          wstunnel_port: srv.wstunnel_port || 0,
        }, srv.serv_addr)
        $clientProcIds = { ...$clientProcIds, [index]: result.procId }
        if (result.localAddr) {
          $clientAddrs = { ...$clientAddrs, [index]: result.localAddr }
          if (onAddLog) onAddLog({ cmd: '客户端', line: `${srv.name}: 已连接, 本地地址 ${result.localAddr}` })
        } else if (result.connAddr) {
          $clientAddrs = { ...$clientAddrs, [index]: result.connAddr }
          if (onAddLog) onAddLog({ cmd: '客户端', line: `${srv.name}: 连接地址 ${result.connAddr}` })
        }
      } catch (e) {
        if (onAddLog) onAddLog({ cmd: '客户端', line: `${srv.name}: 错误: ${e}` })
      }
    }
    loadingIndex = -1
    if (onStatesChange) await onStatesChange()
  }

  $: _anyClientRunning = Object.values($clientProcIds).some(pid => pid != null && states.some(s => s.index === pid && s.status === 'running'))
  $: statusText = _anyClientRunning ? '已连接' : '未连接'
  $: statusColor = _anyClientRunning ? 'var(--green)' : 'var(--text-muted)'

  $: if ($triggerImport) {
    handleImport()
    triggerImport.set(false)
  }
</script>

<div class="client-tab">
  <div class="section">
    <div class="control-bar">
      <div class="btn-group">
        <button class="btn-primary" on:click={handleConnectAll} disabled={processingAll || clientServices.length === 0 || !clientServices.some(s => s.enabled)}>▶ 全部连接</button>
        <button class="btn-danger" on:click={handleDisconnectAll} disabled={processingAll}>⏹ 全部断开</button>
      </div>
      <div class="control-bar-status">
        <span class="status-dot" class:running={_anyClientRunning}></span>
        <span class="status-text" style="color: {statusColor}">{statusText}</span>
      </div>
    </div>
  </div>

  <div class="section">
    <div class="section-title-row">
      <div class="section-title">游戏服务列表</div>
    </div>
    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th class="col-name">游戏名</th>
            <th class="col-conn">连接地址（点击可复制）</th>
            <th class="col-status">状态</th>
            <th class="col-type">类型</th>
            <th class="col-enabled">启用</th>
            <th class="col-action">操作</th>
          </tr>
        </thead>
        <tbody>
          {#each clientServices as srv, i}
            {@const _pid = procIdFor(i)}
            {@const _state = _pid != null ? (states.find(s => s.index === _pid)?.status) || 'stopped' : 'stopped'}
            {@const _running = _state === 'running'}
            <tr>
              <td class="col-name-cell">
                <span class="srv-name">{srv.name}</span>
              </td>
              <td class="col-conn-cell">
                <span class="conn-addr" on:click={() => {const addr = $clientAddrs[i] || connAddr(srv, i); if (addr) {navigator.clipboard.writeText(addr); if (onToast) onToast('已复制地址')}}} title="点击复制">{$clientAddrs[i] || connAddr(srv, i) || '-'}</span>
              </td>
              <td class="col-status-cell">
                {#if srv.connect_method === 'raw'}
                  <span class="text-muted">无需连接</span>
                {:else}
                   <span class="status-dot" class:running={_running}></span>
                  <span class="status-label">{statusLabel(_state)}</span>
                {/if}
              </td>
              <td class="col-type-cell">
                <span class="srv-type">{connTypeLabel(srv)}</span>
              </td>
              <td class="col-enabled-cell">
                <input type="checkbox" checked={srv.enabled} on:change={() => toggleEnabled(i)} disabled={_running} />
              </td>
              <td>
                <div class="btn-group">
                  <button class="btn-sm btn-toggle" class:btn-primary={!_running} class:btn-danger={_running}
                    on:click={() => toggleService(i)} disabled={loadingIndex >= 0 || !srv.enabled || srv.connect_method === 'raw' || !srv.serv_addr}>
                    {loadingIndex === i ? '处理中...' : _running ? '断开' : '连接'}
                  </button>
                  <button class="btn-action" on:click={() => openSettings(i)}>⚙️设置</button>
                  <button class="btn-action" on:click={() => exportService(i)}>📤导出</button>
                  <button class="btn-ghost btn-sm" on:click={() => removeService(i)}
                    disabled={_running}>✕</button>
                </div>
              </td>
            </tr>
          {:else}
            <tr>
              <td colspan="6" class="empty-row">暂无服务，点击"📥 导入连接码"或"+ 添加服务"添加</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <div class="table-actions">
      <button class="btn-primary btn-sm btn-import-auto" on:click={handleImport}>📥 导入连接码</button>
      <button class="btn-ghost btn-sm" on:click={addClientService}>+ 添加服务</button>
    </div>
  </div>

  <LogPanel {logs} {onClearLogs} />
</div>

{#if showSettingsDialog && settingsService}
  <div class="modal-overlay" on:mousedown|self={closeSettings}>
    <div class="modal-dialog" on:click|stopPropagation>
      <div class="modal-header">
        <span class="modal-title">服务设置 — {settingsService.name}</span>
        <button class="btn-ghost btn-sm" on:click={closeSettings}>✕</button>
      </div>
      <div class="modal-body">
        <div class="modal-form-row">
          <span class="form-label">服务名称</span>
          <input bind:value={settingsService.name} placeholder="游戏名" style="flex:1" on:input={autoSaveSettings} />
        </div>
        <div class="modal-form-row">
          <span class="form-label">服务器</span>
          <input bind:value={settingsService.serv_addr} placeholder="example.com" style="flex:1" on:input={autoSaveSettings} />
        </div>
        <div class="modal-divider"></div>
        <div class="modal-form-row">
          <span class="form-label">协议</span>
          <select bind:value={settingsService.protocol} style="flex:1" on:change={autoSaveSettings}>
            <option value="tcp">TCP</option>
            <option value="udp">UDP</option>
          </select>
        </div>
        <div class="modal-form-row">
          <span class="form-label">远程端口</span>
          <input type="number" bind:value={settingsService.remote_port} style="flex:1" on:input={autoSaveSettings} />
        </div>
        <div class="modal-form-row">
          <span class="form-label">连接方式</span>
          <select bind:value={settingsService.connect_method} style="flex:1" on:change={autoSaveSettings}>
            <option value="raw">直连</option>
            <option value="wstunnel">wstunnel</option>
          </select>
        </div>
        {#if settingsService.connect_method === 'wstunnel'}
          <div class="modal-form-row">
            <span class="form-label">WS端口</span>
            <input type="number" bind:value={settingsService.wstunnel_port} style="flex:1" on:input={autoSaveSettings} />
          </div>
        {/if}
      </div>
      <div class="modal-footer">
        <button class="btn-primary" on:click={closeSettings}>关闭</button>
      </div>
    </div>
  </div>
{/if}

{#if showImportDialog}
  <div class="modal-overlay" on:mousedown|self={closeImportDialog}>
    <div class="modal-dialog" on:click|stopPropagation>
      <div class="modal-header">
        <span class="modal-title">导入连接码</span>
        <button class="btn-ghost btn-sm" on:click={closeImportDialog}>✕</button>
      </div>
      <div class="modal-body">
        <div class="modal-form-row">
          <span class="form-label">连接码</span>
          <input bind:value={importCode} on:input={onImportCodeInput} placeholder="slink://..." style="flex:1;font-family:monospace;font-size:12px" />
          <button class="btn-ghost btn-sm" on:click={pasteFromClipboard} title="粘贴">📋</button>
        </div>
        {#if importPreview}
          <div class="import-preview-box">
            <div class="result-row">
              <span class="result-label">名称</span>
              <span class="result-value">{importPreview.n}</span>
            </div>
            <div class="result-row">
              <span class="result-label">协议</span>
              <span class="result-value">{importPreview.p === 'tcp' ? 'TCP' : 'UDP'}</span>
            </div>
            <div class="result-row">
              <span class="result-label">连接方式</span>
              <span class="result-value">{importPreview.c === 'raw' ? '直连' : 'wstunnel'}</span>
            </div>
            <div class="result-row">
              <span class="result-label">目标</span>
              <span class="result-value">{importPreview.rh}:{importPreview.rp}</span>
            </div>
            {#if importPreview.c === 'raw'}
              <div class="result-row">
                <span class="result-label">方式</span>
                <span class="result-value" style="color:var(--accent)">直连地址：{importPreview.rh}:{importPreview.rp}</span>
              </div>
            {:else}
              <div class="result-row">
                <span class="result-label">方式</span>
                <span class="result-value" style="color:var(--accent)">通过 wstunnel 连接</span>
              </div>
              {#if importPreview.wp}
                <div class="result-row">
                  <span class="result-label">wstunnel端口</span>
                  <span class="result-value" style="color:var(--accent)">{importPreview.wp}</span>
                </div>
              {/if}
            {/if}

          </div>
        {/if}
        {#if importError}
          <div class="import-error">{importError}</div>
        {/if}
      </div>
      <div class="modal-footer">
        <button class="btn-ghost" on:click={closeImportDialog}>取消</button>
        <button class="btn-primary" on:click={confirmImport} disabled={!importPreview || importing}>
          {importing ? '导入中...' : '确认导入'}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .client-tab {
    height: 100%;
    display: flex;
    flex-direction: column;
    padding: 12px 20px;
    gap: 12px;
    overflow: hidden;
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
  .col-conn { min-width: 200px; }
  .col-status { width: 100px; }
  .col-action { width: 240px; }

  .srv-name {
    font-weight: 500;
  }

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

  .conn-addr {
    font-family: 'Cascadia Code', 'Fira Code', 'Consolas', monospace;
    font-size: 13px;
    color: var(--accent);
    cursor: pointer;
    user-select: all;
    -webkit-user-select: all;
    white-space: nowrap;
    font-weight: 500;
  }

  .conn-addr:hover {
    text-decoration: underline;
  }

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
  .col-conn-cell {
    text-align: center;
    vertical-align: middle;
  }
  .col-status-cell {
    text-align: center;
    vertical-align: middle;
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

  .btn-group {
    display: flex;
    gap: 6px;
    align-items: center;
  }

  .import-preview-box {
    background: var(--bg-card);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .import-error {
    color: var(--red);
    font-size: 13px;
    padding: 6px 10px;
    background: var(--bg-card);
    border: 1px solid var(--red);
    border-radius: var(--radius);
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
    width: 520px;
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
    width: 80px;
    flex-shrink: 0;
  }

  .modal-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 12px 18px;
    border-top: 1px solid var(--border);
  }

  .modal-divider {
    height: 1px;
    background: var(--border);
    margin: 4px 0;
  }

  .result-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .result-label {
    color: var(--text-secondary);
    font-size: 13px;
    width: 80px;
    flex-shrink: 0;
  }

  .result-value {
    font-size: 13px;
    font-weight: 500;
  }

  .modal-body select {
    padding: 6px 10px;
    background: var(--bg-card);
    border: 1px solid var(--border);
    font-size: 13px;
  }

  .modal-body input[type="number"] {
    padding: 6px 10px;
    font-size: 13px;
  }

  .table-actions {
    display: flex;
    gap: 8px;
    margin-top: 8px;
    align-items: center;
  }

  .btn-import-auto {
    width: 120px;
  }
</style>
