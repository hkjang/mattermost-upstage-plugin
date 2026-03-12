import type {WebSocketMessage} from '@mattermost/client';

import type {StreamingPostUpdateEventData} from './streaming';

type PostUpdateListener = (msg: WebSocketMessage<StreamingPostUpdateEventData>) => void;

export default class PostEventListener {
    private readonly listeners = new Map<string, Map<string, PostUpdateListener>>();

    public registerPostUpdateListener = (postID: string, listenerID: string, listener: PostUpdateListener) => {
        const normalizedPostID = normalizePostID(postID);
        const normalizedListenerID = normalizePostID(listenerID);
        if (!normalizedPostID || !normalizedListenerID) {
            return;
        }

        const postListeners = this.listeners.get(normalizedPostID) || new Map<string, PostUpdateListener>();
        postListeners.set(normalizedListenerID, listener);
        this.listeners.set(normalizedPostID, postListeners);
    };

    public unregisterPostUpdateListener = (postID: string, listenerID: string) => {
        const normalizedPostID = normalizePostID(postID);
        const normalizedListenerID = normalizePostID(listenerID);
        if (!normalizedPostID || !normalizedListenerID) {
            return;
        }

        const postListeners = this.listeners.get(normalizedPostID);
        if (!postListeners) {
            return;
        }

        postListeners.delete(normalizedListenerID);
        if (postListeners.size === 0) {
            this.listeners.delete(normalizedPostID);
        }
    };

    public handlePostUpdateWebsockets = (msg: WebSocketMessage<StreamingPostUpdateEventData>) => {
        const postID = normalizePostID(msg?.data?.post_id);
        if (!postID) {
            return;
        }

        const listeners = this.listeners.get(postID);
        if (!listeners || listeners.size === 0) {
            return;
        }

        for (const listener of listeners.values()) {
            listener(msg);
        }
    };
}

function normalizePostID(value?: string) {
    if (typeof value !== 'string') {
        return '';
    }

    return value.trim();
}
