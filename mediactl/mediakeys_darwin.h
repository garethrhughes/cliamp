#ifndef MEDIAKEYS_DARWIN_H
#define MEDIAKEYS_DARWIN_H

#include <stdbool.h>

// MediaKeysInit registers MPRemoteCommandCenter handlers.
// Must be called before MediaKeysRunLoop.
void MediaKeysInit(void);

// MediaKeysUpdateNowPlaying pushes track metadata and playback state to
// MPNowPlayingInfoCenter. Pass NULL for strings to leave them unset.
void MediaKeysUpdateNowPlaying(const char *title, const char *artist,
                               const char *album, double durationSec,
                               double positionSec, bool playing);

// MediaKeysUpdatePosition updates just the elapsed position (seconds)
// in the Now Playing info, leaving other fields unchanged.
void MediaKeysUpdatePosition(double positionSec);

// MediaKeysClose removes all command targets and clears Now Playing info.
void MediaKeysClose(void);

#endif /* MEDIAKEYS_DARWIN_H */
