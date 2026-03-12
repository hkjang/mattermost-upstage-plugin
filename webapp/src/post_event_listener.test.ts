import type {WebSocketMessage} from '@mattermost/client';

import PostEventListener from './post_event_listener';

test('PostEventListener notifies every listener registered for the same post', () => {
    const listener = new PostEventListener();
    const first = jest.fn();
    const second = jest.fn();
    const message = {
        data: {
            post_id: 'post-id',
            next: 'chunk',
        },
    } as WebSocketMessage<{post_id: string; next: string}>;

    listener.registerPostUpdateListener('post-id', 'first', first);
    listener.registerPostUpdateListener('post-id', 'second', second);
    listener.handlePostUpdateWebsockets(message as any);

    expect(first).toHaveBeenCalledTimes(1);
    expect(second).toHaveBeenCalledTimes(1);
});

test('PostEventListener unregisters one listener without affecting others', () => {
    const listener = new PostEventListener();
    const first = jest.fn();
    const second = jest.fn();
    const message = {
        data: {
            post_id: 'post-id',
            next: 'chunk',
        },
    } as WebSocketMessage<{post_id: string; next: string}>;

    listener.registerPostUpdateListener('post-id', 'first', first);
    listener.registerPostUpdateListener('post-id', 'second', second);
    listener.unregisterPostUpdateListener('post-id', 'first');
    listener.handlePostUpdateWebsockets(message as any);

    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledTimes(1);
});
