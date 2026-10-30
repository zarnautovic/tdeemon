import { queryOptions } from '@tanstack/react-query'

import { sync } from '@/lib/push'

// pushDeviceQuery is this device's notifications (lib/push), read once
// the app opens, which tells the server of its subscription again; the
// settings change it in place.
export const pushDeviceQuery = queryOptions({
  queryKey: ['push', 'device'],
  queryFn: () => sync(),
  staleTime: Number.POSITIVE_INFINITY,
  retry: false,
})
