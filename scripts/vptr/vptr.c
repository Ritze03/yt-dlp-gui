#include <wayland-client.h>
#include <linux/input-event-codes.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include "vp.h"
static struct zwlr_virtual_pointer_manager_v1 *mgr;
static struct wl_seat *seat;
static void reg(void *d, struct wl_registry *r, uint32_t n, const char *i, uint32_t v) {
  if (!strcmp(i, zwlr_virtual_pointer_manager_v1_interface.name)) mgr = wl_registry_bind(r, n, &zwlr_virtual_pointer_manager_v1_interface, 1);
  if (!strcmp(i, "wl_seat") && !seat) seat = wl_registry_bind(r, n, &wl_seat_interface, 1);
}
static void rem(void *d, struct wl_registry *r, uint32_t n) {}
static const struct wl_registry_listener rl = {reg, rem};
int main(int argc, char **argv) {
  int x = atoi(argv[1]), y = atoi(argv[2]);
  struct wl_display *dpy = wl_display_connect(NULL);
  struct wl_registry *r = wl_display_get_registry(dpy);
  wl_registry_add_listener(r, &rl, NULL);
  wl_display_roundtrip(dpy);
  struct zwlr_virtual_pointer_v1 *p = zwlr_virtual_pointer_manager_v1_create_virtual_pointer(mgr, seat);
  zwlr_virtual_pointer_v1_motion_absolute(p, 0, x, y, 1100, 750);
  zwlr_virtual_pointer_v1_frame(p);
  wl_display_roundtrip(dpy); usleep(200000);
  if (argc > 3) {
    zwlr_virtual_pointer_v1_button(p, 1, BTN_LEFT, WL_POINTER_BUTTON_STATE_PRESSED);
    zwlr_virtual_pointer_v1_frame(p);
    wl_display_roundtrip(dpy); usleep(100000);
    zwlr_virtual_pointer_v1_button(p, 2, BTN_LEFT, WL_POINTER_BUTTON_STATE_RELEASED);
    zwlr_virtual_pointer_v1_frame(p);
    wl_display_roundtrip(dpy); usleep(100000);
  }
  zwlr_virtual_pointer_v1_destroy(p);
  wl_display_roundtrip(dpy);
  return 0;
}
