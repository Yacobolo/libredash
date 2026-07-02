import { css } from 'lit'

export const visualActionStyles = css`
  .visual-actions {
    display: flex;
    flex: 0 0 auto;
    align-items: center;
    gap: var(--base-size-4);
  }

  .icon-action {
    display: grid;
    width: var(--ld-button-height-xs, var(--control-xsmall-size));
    height: var(--ld-button-height-xs, var(--control-xsmall-size));
    min-height: var(--ld-button-height-xs, var(--control-xsmall-size));
    place-items: center;
    border: var(--borderWidth-default, var(--ld-border-width)) solid var(--ld-button-invisible-border-rest, var(--control-transparent-borderColor-rest, var(--ld-line-muted)));
    border-radius: var(--ld-radius-tight);
    background: var(--ld-button-invisible-bg-rest, var(--control-transparent-bgColor-rest, var(--ld-bg-panel)));
    color: var(--ld-button-invisible-icon-rest, var(--ld-icon-muted, var(--ld-fg-muted)));
    cursor: pointer;
    padding: 0;
    font: inherit;
    line-height: var(--ld-line-height-none);
  }

  .icon-action svg {
    width: var(--base-size-16);
    height: var(--base-size-16);
  }

  .icon-action:hover,
  .icon-action:focus-visible {
    border-color: var(--ld-button-invisible-border-hover, var(--control-transparent-borderColor-hover, var(--ld-line-default)));
    background: var(--ld-button-invisible-bg-hover, var(--control-transparent-bgColor-hover, var(--ld-bg-panel-muted)));
    color: var(--ld-icon-default, var(--ld-fg-default));
    outline: var(--focus-outline, var(--ld-border-default));
    outline-color: var(--borderColor-accent-emphasis, var(--ld-line-accent));
    outline-offset: var(--focus-outline-offset, var(--base-size-2));
  }
`
