// coreaudio_tap.h - CoreAudio Tap API wrapper header
#ifndef COREAUDIO_TAP_H
#define COREAUDIO_TAP_H

#include <stdint.h>

typedef struct {
    uint32_t tapID;
    uint32_t aggregateDeviceID;
    int error;
} TapResult;

TapResult createGlobalTap();
void destroyTap(uint32_t tapID, uint32_t aggregateDeviceID);
void getDeviceName(uint32_t deviceID, char *buf, int bufSize);

#endif
