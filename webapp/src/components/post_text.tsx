import React, {useEffect, useMemo} from 'react';
import {useSelector} from 'react-redux';

import type {Channel} from '@mattermost/types/channels';
import type {GlobalState} from '@mattermost/types/store';
import type {Team} from '@mattermost/types/teams';

import MermaidDiagram from './mermaid_diagram';

import {splitRenderableMessage} from '../mermaid_rendering';

const cursorClassName = 'upstage-streaming-post-cursor';
const markdownBodyClassName = 'upstage-markdown-body';
const mattermostPostTextClassName = 'post-message__text';

const containerStyle: React.CSSProperties = {
    display: 'block',
    maxWidth: '100%',
    overflow: 'hidden',
    wordBreak: 'break-word',
};

let streamingStylesInjected = false;

type Props = {
    message: string;
    channelID: string;
    postID: string;
    showCursor?: boolean;
};

type PostUtils = {
    formatText: (value: string, options: Record<string, unknown>) => string;
    messageHtmlToComponent: (value: string, options: Record<string, unknown>) => React.ReactNode;
};

export default function PostText({message, channelID, postID, showCursor}: Props) {
    const channel = useSelector<GlobalState, Channel | undefined>((state) => state.entities.channels.channels[channelID]);
    const team = useSelector<GlobalState, Team | undefined>((state) => state.entities.teams.teams[channel?.team_id || '']);
    const siteURL = useSelector<GlobalState, string | undefined>((state) => state.entities.general.config.SiteURL);

    useEffect(() => {
        ensureStreamingStyles();
    }, []);

    const segments = useMemo(() => splitRenderableMessage(message), [message]);
    const lastTextSegmentIndex = useMemo(() => {
        for (let index = segments.length - 1; index >= 0; index--) {
            if (segments[index].kind === 'text') {
                return index;
            }
        }
        return -1;
    }, [segments]);

    const postUtils = (window as any).PostUtils as PostUtils | undefined;
    const markdownOptions = {
        singleline: false,
        mentionHighlight: true,
        atMentions: true,
        team,
        unsafeLinks: false,
        minimumHashtagLength: 1000000000,
        siteURL,
        markdown: true,
    };
    const componentOptions = {
        hasPluginTooltips: true,
        latex: false,
        inlinelatex: false,
        postId: postID,
    };

    if (!postUtils) {
        return (
            <div
                className={buildContainerClassName(showCursor)}
                data-testid='posttext'
                style={containerStyle}
            >
                {message}
                {showCursor && <CursorFallback/>}
            </div>
        );
    }

    const renderedSegments = segments.map((segment, index) => {
        if (segment.kind === 'mermaid') {
            return (
                <MermaidDiagram
                    definition={segment.content}
                    index={index}
                    key={`${postID}-mermaid-${index}`}
                    postID={postID}
                />
            );
        }

        if (!segment.content) {
            return null;
        }

        const formattedMessage = postUtils.formatText(segment.content, markdownOptions);
        const content = postUtils.messageHtmlToComponent(formattedMessage, componentOptions);
        if (!content) {
            return index === lastTextSegmentIndex && showCursor ? <p key={`${postID}-empty-${index}`}/> : null;
        }

        return (
            <React.Fragment key={`${postID}-text-${index}`}>
                {content}
            </React.Fragment>
        );
    }).filter(Boolean);

    const shouldRenderFallbackCursor = showCursor && lastTextSegmentIndex < 0;

    return (
        <div
            className={buildContainerClassName(showCursor)}
            data-testid='posttext'
            style={containerStyle}
        >
            {renderedSegments.length > 0 ? renderedSegments : <p/>}
            {shouldRenderFallbackCursor && <CursorFallback/>}
        </div>
    );
}

function CursorFallback() {
    return (
        <span
            style={{
                animation: 'upstage-stream-cursor-blink 500ms ease-in-out infinite',
                background: 'rgba(var(--center-channel-color-rgb), 0.48)',
                display: 'inline-block',
                height: '16px',
                marginLeft: '3px',
                verticalAlign: 'text-bottom',
                width: '7px',
            }}
        />
    );
}

function ensureStreamingStyles() {
    if (streamingStylesInjected || typeof document === 'undefined') {
        return;
    }

    const style = document.createElement('style');
    style.setAttribute('data-upstage-streaming-cursor', 'true');
    style.textContent = `
@keyframes upstage-stream-cursor-blink {
    0% { opacity: 0.48; }
    20% { opacity: 0.48; }
    100% { opacity: 0.12; }
}

.${cursorClassName} > ul:last-child > li:last-child > span:not(:has(li))::after,
.${cursorClassName} > ol:last-child > li:last-child > span:not(:has(li))::after,
.${cursorClassName} > ul:last-child > li:last-child > span > ul > li:last-child > span:not(:has(li))::after,
.${cursorClassName} > ol:last-child > li:last-child > span > ul > li:last-child > span:not(:has(li))::after,
.${cursorClassName} > ul:last-child > li:last-child > span > ol > li:last-child > span:not(:has(li))::after,
.${cursorClassName} > ol:last-child > li:last-child > span > ol > li:last-child > span:not(:has(li))::after,
.${cursorClassName} > h1:last-child::after,
.${cursorClassName} > h2:last-child::after,
.${cursorClassName} > h3:last-child::after,
.${cursorClassName} > h4:last-child::after,
.${cursorClassName} > h5:last-child::after,
.${cursorClassName} > h6:last-child::after,
.${cursorClassName} > blockquote:last-child > p::after,
.${cursorClassName} > p:last-child::after,
.${cursorClassName} > p:empty::after {
    content: '';
    width: 7px;
    height: 16px;
    background: rgba(var(--center-channel-color-rgb), 0.48);
    display: inline-block;
    margin-left: 3px;
    animation: upstage-stream-cursor-blink 500ms ease-in-out infinite;
}

.${markdownBodyClassName} table,
.${markdownBodyClassName} .markdown__table {
    border-collapse: collapse;
    border-spacing: 0;
    display: block;
    margin: 12px 0;
    max-width: 100%;
    min-width: 100%;
    overflow-x: auto;
    width: max-content;
}

.${markdownBodyClassName} thead {
    background: rgba(var(--center-channel-color-rgb), 0.04);
}

.${markdownBodyClassName} th,
.${markdownBodyClassName} td {
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.16);
    padding: 8px 12px;
    vertical-align: top;
    white-space: pre-wrap;
    word-break: break-word;
}

.${markdownBodyClassName} pre,
.${markdownBodyClassName} .post-code {
    max-width: 100%;
    overflow-x: auto;
}

.${markdownBodyClassName} img {
    height: auto;
    max-width: 100%;
}

.${markdownBodyClassName} .upstage-mermaid-rendered {
    min-height: 48px;
    overflow-x: auto;
}

.${markdownBodyClassName} .upstage-mermaid-card {
    background: rgba(var(--center-channel-color-rgb), 0.03);
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.12);
    border-radius: 12px;
    margin: 12px 0;
    max-width: 100%;
    padding: 12px;
    position: relative;
}

.${markdownBodyClassName} .upstage-mermaid-toolbar {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    margin-bottom: 8px;
}

.${markdownBodyClassName} .upstage-mermaid-toolbar-button {
    background: rgba(var(--center-channel-color-rgb), 0.08);
    border: 1px solid rgba(var(--center-channel-color-rgb), 0.14);
    border-radius: 999px;
    color: inherit;
    cursor: pointer;
    font-size: 12px;
    font-weight: 600;
    line-height: 1;
    padding: 6px 10px;
}

.${markdownBodyClassName} .upstage-mermaid-toolbar-button:hover {
    background: rgba(var(--center-channel-color-rgb), 0.12);
}

.${markdownBodyClassName} .upstage-mermaid-error {
    color: var(--dnd-indicator);
    font-size: 12px;
    font-weight: 600;
    margin-bottom: 8px;
}

.${markdownBodyClassName} .upstage-mermaid-rendered svg {
    display: block;
    height: auto;
    margin: 0 auto;
    max-width: 100%;
}

.${markdownBodyClassName} .upstage-mermaid-fallback {
    margin: 0;
}

.${markdownBodyClassName} .upstage-mermaid-modal-backdrop {
    align-items: center;
    background: rgba(0, 0, 0, 0.48);
    display: flex;
    inset: 0;
    justify-content: center;
    padding: 20px;
    position: fixed;
    z-index: 1000;
}

.${markdownBodyClassName} .upstage-mermaid-modal {
    background: var(--center-channel-bg);
    border-radius: 16px;
    box-shadow: 0 18px 48px rgba(0, 0, 0, 0.24);
    max-height: min(80vh, 720px);
    max-width: min(900px, 100%);
    overflow: hidden;
    width: min(900px, 100%);
}

.${markdownBodyClassName} .upstage-mermaid-render-modal {
    max-width: min(1100px, 100%);
    width: min(1100px, 100%);
}

.${markdownBodyClassName} .upstage-mermaid-modal-header {
    align-items: center;
    border-bottom: 1px solid rgba(var(--center-channel-color-rgb), 0.12);
    display: flex;
    justify-content: space-between;
    padding: 16px 18px;
}

.${markdownBodyClassName} .upstage-mermaid-modal-actions {
    display: flex;
    gap: 8px;
}

.${markdownBodyClassName} .upstage-mermaid-source {
    margin: 0;
    max-height: calc(80vh - 72px);
    overflow: auto;
    padding: 18px;
}

.${markdownBodyClassName} .upstage-mermaid-modal-content {
    max-height: calc(80vh - 72px);
    overflow: auto;
    padding: 18px;
}

.${markdownBodyClassName} .upstage-mermaid-modal-error {
    margin: 16px 18px 0;
}

.${markdownBodyClassName} .upstage-mermaid-rendered-popup {
    min-height: 320px;
}
`;
    document.head.appendChild(style);
    streamingStylesInjected = true;
}

function buildContainerClassName(showCursor?: boolean) {
    return [
        mattermostPostTextClassName,
        markdownBodyClassName,
        showCursor ? cursorClassName : '',
    ].filter(Boolean).join(' ');
}
