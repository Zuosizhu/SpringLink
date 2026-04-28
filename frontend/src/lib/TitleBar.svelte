<script>
  import { WindowMinimise, WindowToggleMaximise } from '../../wailsjs/runtime/runtime.js'
  import { MinimizeToTray } from '../../wailsjs/go/main/App.js'

  export let activeTab = 'server'
  export let onSwitchTab = () => {}
  export let darkMode = true
  export let onToggleTheme = () => {}
  export let onClose = () => {}

  let maximised = false
</script>

<div class="titlebar" style="--wails-draggable: drag">
  <div class="titlebar-drag">
    <div class="titlebar-title">SpringLink</div>
    <div class="titlebar-tabs">
      <button class="tab-btn" class:active={activeTab === 'server'} on:click={() => onSwitchTab('server')} style="--wails-draggable: none">
        服务端
      </button>
      <button class="tab-btn" class:active={activeTab === 'client'} on:click={() => onSwitchTab('client')} style="--wails-draggable: none">
        客户端
      </button>
    </div>
  </div>
  <div class="titlebar-controls" style="--wails-draggable: none">
    <button class="ctrl-btn ctrl-tray" on:click={MinimizeToTray} title="最小化到托盘">
      <svg viewBox="0 0 16 16" style="width:14px;height:14px"><path d="M2 13 L14 13 M8 3 L8 10 M5 7 L8 10 L11 7" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg>
    </button>
    <button class="ctrl-btn ctrl-theme" on:click={onToggleTheme} title={darkMode ? '切换亮色模式' : '切换暗色模式'}>
      <svg class="bulb-icon" class:active={!darkMode} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="width:14px;height:14px">
        <path d="M9 18h6"/>
        <path d="M10 22h4"/>
        <path d="M15.09 14c.18-.98.65-1.74 1.41-2.5A4.65 4.65 0 0 0 18 8 6 6 0 0 0 6 8c0 1 .23 2.23 1.5 3.5A4.61 4.61 0 0 1 8.91 14"/>
      </svg>
    </button>
    <button class="ctrl-btn" on:click={WindowMinimise} title="最小化">
      <svg viewBox="0 0 12 12"><rect y="5" width="12" height="1.5" fill="currentColor"/></svg>
    </button>
    <button class="ctrl-btn" on:click={async () => { WindowToggleMaximise(); maximised = !maximised }} title={maximised ? '还原' : '最大化'}>
      {#if maximised}
        <svg viewBox="0 0 12 12"><rect x="2" y="0" width="8" height="8" fill="none" stroke="currentColor" stroke-width="1.2"/><rect x="1" y="2" width="8" height="8" fill="none" stroke="currentColor" stroke-width="1.2"/></svg>
      {:else}
        <svg viewBox="0 0 12 12"><rect x="1.5" y="1.5" width="9" height="9" fill="none" stroke="currentColor" stroke-width="1.2"/></svg>
      {/if}
    </button>
    <button class="ctrl-btn ctrl-close" on:click={onClose} title="关闭">
      <svg viewBox="0 0 12 12"><path d="M2 2 L10 10 M10 2 L2 10" stroke="currentColor" stroke-width="1.5" fill="none"/></svg>
    </button>
  </div>
</div>

<style>
  .titlebar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 36px;
    background: var(--bg-secondary);
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }

  .titlebar-drag {
    display: flex;
    align-items: center;
    gap: 16px;
    height: 100%;
    flex: 1;
  }

  .titlebar-title {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-muted);
    padding-left: 14px;
    letter-spacing: 0.5px;
  }

  .titlebar-tabs {
    display: flex;
    gap: 2px;
    height: 100%;
    align-items: stretch;
  }

  .tab-btn {
    background: transparent;
    color: var(--text-muted);
    padding: 0 14px;
    font-size: 12px;
    border-radius: 0;
    border-bottom: 2px solid transparent;
    height: 100%;
    cursor: pointer;
  }

  .tab-btn:hover {
    color: var(--text-primary);
    background: var(--bg-hover);
  }

  .tab-btn.active {
    color: var(--accent);
    border-bottom-color: var(--accent);
  }

  .titlebar-controls {
    display: flex;
    height: 100%;
  }

  .ctrl-btn {
    background: transparent;
    color: var(--text-muted);
    width: 46px;
    height: 100%;
    padding: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 0;
    cursor: pointer;
  }

  .ctrl-btn:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }

  .ctrl-close:hover {
    background: var(--red);
    color: #fff;
  }

  .ctrl-btn svg {
    width: 12px;
    height: 12px;
  }
</style>
