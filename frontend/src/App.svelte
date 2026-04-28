<script>
  import { onMount, onDestroy } from 'svelte'
  import { GetConfig, GetServerStates, GetClientStates, SaveConfig, MinimizeToTray, CancelClose } from '../wailsjs/go/main/App.js'
  import { triggerImport } from './lib/clientStore.js'
  import { triggerPublicIP, publicIPClosedSig } from './lib/serverStore.js'
  import { EventsOn, EventsOff, Quit, Show } from '../wailsjs/runtime/runtime.js'
  import TitleBar from './lib/TitleBar.svelte'
  import ServerTab from './lib/ServerTab.svelte'
  import ClientTab from './lib/ClientTab.svelte'

  let activeTab = 'server'
  let darkMode = true
  let config

  function detectTheme() {
    darkMode = window.matchMedia('(prefers-color-scheme: dark)').matches
  }
  let serverStates = []
  let clientStates = []
  let serverLogs = []
  let clientLogs = []

  function toggleTheme() {
    darkMode = !darkMode
  }

  function addServerLog(entry) {
    serverLogs = [...serverLogs.slice(-500), entry]
  }

  function addClientLog(entry) {
    clientLogs = [...clientLogs.slice(-500), entry]
  }

  function clearServerLogs() {
    serverLogs = []
  }

  function clearClientLogs() {
    clientLogs = []
  }

  let showCloseConfirm = false
  let closing = false

  $: anyRunning = serverStates.some(s => s.status === 'running' || s.status === 'starting') || clientStates.some(s => s.status === 'running' || s.status === 'starting')

  function handleClose() {
    if (closing) {
      Quit()
      return
    }
    if (anyRunning) {
      showCloseConfirm = true
    } else {
      closing = true
      Quit()
    }
  }

	async function confirmCloseAndQuit() {
		showCloseConfirm = false
		closing = true
		Quit()
	}

	function cancelClose() {
		showCloseConfirm = false
		closing = false
		CancelClose()
	}

	function handleMinimizeToTray() {
		showCloseConfirm = false
		closing = false
		CancelClose()
		MinimizeToTray()
	}

  let toast = { show: false, message: '' }
  let toastTimer
  let showGuide = false
  let guideShown = false
  let guideStep = 'role'
  let guideRole = null
  let guideHasPublicIp = null
  let guideChinaMobile = null
  let guideResumeStep = null

  function showToast(msg) {
    clearTimeout(toastTimer)
    toast = { show: true, message: msg }
    toastTimer = setTimeout(() => toast = { show: false, message: '' }, 2000)
  }

  async function loadConfig() {
    config = await GetConfig()
    if (!guideShown && config && !config.general.guided) {
      const hasServerServices = config.services && config.services.length > 0
      const hasClientServices = config.client && config.client.services && config.client.services.length > 0
      if (!hasServerServices && !hasClientServices) {
        showGuide = true
        guideShown = true
      }
    }
    if (config.general.active_tab === 'server' || config.general.active_tab === 'client') {
      activeTab = config.general.active_tab
    }
  }

  async function switchTab(tab) {
    activeTab = tab
    if (config) {
      config.general.active_tab = tab
      await SaveConfig(config)
    }
  }

  async function refreshServerStates() {
    serverStates = await GetServerStates()
  }

  async function refreshClientStates() {
    clientStates = await GetClientStates()
  }

	onMount(async () => {
		detectTheme()
		const mq = window.matchMedia('(prefers-color-scheme: dark)')
		mq.addEventListener('change', detectTheme)

		await loadConfig()
		refreshServerStates()
		refreshClientStates()

		EventsOn('service-log', (data) => {
			if (data.type === 'server') {
				addServerLog(data)
			} else {
				addClientLog(data)
			}
		})

		EventsOn('service-state-changed', () => {
			refreshServerStates()
			refreshClientStates()
		})

		EventsOn('tray-quit-requested', () => {
			Show()
			handleClose()
		})

		EventsOn('window-close-requested', () => {
			handleClose()
		})
	})

	onDestroy(() => {
		EventsOff('service-log')
		EventsOff('service-state-changed')
		EventsOff('tray-quit-requested')
		EventsOff('window-close-requested')
	})

  $: if ($publicIPClosedSig > 0 && guideResumeStep) {
    showGuide = true
    guideStep = guideResumeStep
    guideResumeStep = null
  }

function closeGuide() {
  showGuide = false
  if (config) {
    config.general.guided = true
    SaveConfig(config)
  }
}

async function handleGuideServer() {
  guideRole = 'server'
  guideStep = 'public-ip'
}

async function handleGuidePlayer() {
  showGuide = false
  activeTab = 'client'
  if (config) {
    config.general.guided = true
    config.general.active_tab = 'client'
    await SaveConfig(config)
  }
  triggerImport.set(true)
}

async function handleGuidePublicIP(value) {
  guideHasPublicIp = value
  if (value) {
    if (config) {
      config.general.has_public_ip = true
      await SaveConfig(config)
    }
    guideResumeStep = 'isp-advice'
    showGuide = false
    activeTab = 'server'
    triggerPublicIP.update(n => n + 1)
  } else {
    if (config) {
      config.general.has_public_ip = false
      config.general.public_ip = ''
      await SaveConfig(config)
    }
    guideStep = 'isp-advice'
  }
}

function handleGuideChinaMobile(value) {
  guideChinaMobile = value
}

function finishGuide() {
  if (config) {
    config.general.guided = true
    SaveConfig(config)
  }
  showGuide = false
}
</script>

<div class="app-container" data-theme={darkMode ? 'dark' : 'light'}>
  <TitleBar {activeTab} onSwitchTab={switchTab} {darkMode} onToggleTheme={toggleTheme} onClose={handleClose} />

  <main class="app-main">
    {#if config}
      {#if activeTab === 'server'}
        <ServerTab
          {config}
          states={serverStates}
          logs={serverLogs}
          onConfigChange={loadConfig}
          onStatesChange={refreshServerStates}
          onClearLogs={clearServerLogs}
          onAddLog={addServerLog}
          onToast={showToast}
        />
      {:else}
        <ClientTab
          {config}
          logs={clientLogs}
          states={clientStates}
          onClearLogs={clearClientLogs}
          onConfigChange={loadConfig}
          onStatesChange={refreshClientStates}
          onAddLog={addClientLog}
          onToast={showToast}
        />
      {/if}
    {:else}
      <div class="loading">加载配置中...</div>
    {/if}
  </main>

  {#if showGuide}
  <div class="modal-overlay guide-overlay" on:mousedown|self={closeGuide}>
    <div class="modal-dialog guide-dialog" on:click|stopPropagation>
      <div class="modal-header">
      </div>
      <div class="modal-body">
        {#if guideStep === 'role'}
          <div class="guide-section">
            <div class="guide-question">你是主机还是玩家？</div>
            <div class="guide-btn-row">
              <button class="guide-option-btn" on:click={handleGuideServer}>主机</button>
              <button class="guide-option-btn" on:click={handleGuidePlayer}>玩家</button>
            </div>
          </div>
        {:else if guideStep === 'public-ip'}
          <div class="guide-section">
            <div class="guide-question">您有公网IP吗？</div>
            <div class="guide-btn-row">
              <button class="guide-option-btn" class:active={guideHasPublicIp === true} on:click={() => handleGuidePublicIP(true)}>是</button>
              <button class="guide-option-btn" class:active={guideHasPublicIp === false} on:click={() => handleGuidePublicIP(false)}>否</button>
            </div>
          </div>
        {:else if guideStep === 'isp-advice'}
          <div class="guide-section">
            <div class="guide-question">您和您的玩家有使用中国移动（或经历过UDP联机卡顿）的吗？</div>
            <div class="guide-btn-row">
              <button class="guide-option-btn" class:active={guideChinaMobile === true} on:click={() => handleGuideChinaMobile(true)}>是</button>
              <button class="guide-option-btn" class:active={guideChinaMobile === false} on:click={() => handleGuideChinaMobile(false)}>否</button>
            </div>
            {#if guideChinaMobile === true}
              <div class="guide-advice-box">
                建议选择包含wstunnel的方案，注意：frp中继+wstunnel延迟非常严重，但这是抗Qos最强的方案
              </div>
            {:else if guideChinaMobile === false}
              <div class="guide-advice-box">
                建议选择仅frp或wstunnel的单纯穿透方案，玩家不使用wstunnel连接
              </div>
            {/if}
          </div>
        {/if}
      </div>
      <div class="guide-footer">
        {#if guideStep === 'isp-advice' && guideChinaMobile !== null}
          <button class="guide-skip-btn guide-done-btn" on:click={finishGuide}>完成</button>
        {:else}
          <button class="guide-skip-btn" on:click={closeGuide}>跳过</button>
        {/if}
      </div>
    </div>
  </div>
{/if}

  {#if toast.show}
    <div class="toast">{toast.message}</div>
  {/if}

  {#if showCloseConfirm}
    <div class="modal-overlay" on:mousedown|self={cancelClose}>
      <div class="modal-dialog close-confirm-dialog" on:click|stopPropagation>
        <div class="close-confirm-body">
          <div class="close-confirm-title">确认关闭</div>
          <div class="close-confirm-msg">关闭界面会关闭所有服务与游戏连接，是否继续？</div>
          <div class="close-confirm-actions">
            <button class="btn-danger" on:click={confirmCloseAndQuit}>关闭</button>
            <button class="btn-primary" on:click={handleMinimizeToTray}>最小化到托盘</button>
            <button class="btn-ghost" on:click={cancelClose}>取消</button>
          </div>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .app-container {
    height: 100vh;
    display: flex;
    flex-direction: column;
    background: var(--bg-primary);
    color: var(--text-primary);
  }

  .app-main {
    flex: 1;
    overflow: hidden;
  }

  .loading {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    color: var(--text-muted);
  }

  .toast {
    position: fixed;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    background: var(--bg-card);
    color: var(--text-primary);
    padding: 16px 32px;
    border-radius: var(--radius);
    border: 1px solid var(--accent);
    box-shadow: 0 4px 20px rgba(0,0,0,0.4);
    font-size: 14px;
    font-weight: 600;
    z-index: 9999;
    animation: toast-in 0.2s ease-out;
  }

  @keyframes toast-in {
    from {
      opacity: 0;
      transform: translate(-50%, -50%) scale(0.9);
    }
    to {
      opacity: 1;
      transform: translate(-50%, -50%) scale(1);
    }
  }

  .close-confirm-dialog {
    width: 400px;
    background: var(--bg-primary);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: var(--shadow);
  }

  .close-confirm-body {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 28px 24px 20px;
  }

  .close-confirm-title {
    font-size: 16px;
    font-weight: 600;
  }

  .close-confirm-msg {
    color: var(--text-secondary);
    font-size: 13px;
    text-align: center;
    line-height: 1.5;
  }

  .close-confirm-actions {
    display: flex;
    justify-content: center;
    gap: 10px;
    margin-top: 4px;
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

.guide-overlay {
  z-index: 2000;
}

.guide-dialog {
  width: 480px;
  max-height: 70vh;
  background: var(--bg-primary);
}

.guide-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 20px;
  padding: 16px 0;
}

.guide-question {
  font-size: 16px;
  font-weight: 600;
  text-align: center;
  line-height: 1.5;
}

.guide-btn-row {
  display: flex;
  gap: 20px;
  justify-content: center;
}

.guide-option-btn {
  width: 140px;
  height: 80px;
  font-size: 18px;
  font-weight: 600;
  background: var(--bg-card);
  border: 2px solid var(--border);
  border-radius: var(--radius);
  color: var(--text-primary);
  cursor: pointer;
  transition: all 0.15s;
}

.guide-option-btn:hover {
  border-color: var(--accent);
  background: var(--accent-dim);
}

.guide-option-btn.active {
  border-color: var(--accent);
  background: var(--accent-dim);
}

.guide-advice-box {
  background: var(--bg-card);
  border: 1px solid var(--accent);
  border-radius: var(--radius);
  padding: 14px 18px;
  font-size: 13px;
  line-height: 1.6;
  color: var(--text-primary);
  max-width: 400px;
}

.guide-footer {
  display: flex;
  justify-content: center;
  padding: 8px 18px 14px;
  border-top: 1px solid var(--border);
}

.guide-skip-btn {
  background: none;
  border: none;
  color: var(--text-muted);
  font-size: 12px;
  cursor: pointer;
  padding: 4px 12px;
  opacity: 0.5;
  transition: opacity 0.15s;
}

.guide-skip-btn:hover {
  opacity: 1;
  color: var(--text-secondary);
}

.guide-done-btn {
  opacity: 1;
  color: var(--accent);
  font-weight: 600;
  font-size: 13px;
}

.guide-done-btn:hover {
  color: var(--accent);
  opacity: 0.8;
}
</style>
