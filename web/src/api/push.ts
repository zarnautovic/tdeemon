import { queryOptions } from '@tanstack/react-query'

import { sync } from '@/lib/push'

// pushDeviceQuery is this device's notifications (lib/push), read as the
// app opens and as it comes back into view after a while, which tells
// the server of its subscription again and sees a permission changed in
// the browser's settings; the settings change it in place.
export const pushDeviceQuery = queryOptions({
  queryKey: ['push', 'device'],
  queryFn: () => sync(),
  staleTime: 5 * 60 * 1000,
  refetchOnWindowFocus: true,
  retry: false,
})
