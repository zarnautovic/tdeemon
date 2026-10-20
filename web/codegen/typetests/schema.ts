// Type-level checks of the generated types: this file only has to compile
// (npm run check:api). It pins behaviour the generator options decide.
import type { components } from "../../src/api/schema";

// A field with a default is optional. huma leaves such fields out of a
// response when they are empty and fills them in for a request, so a
// client must neither expect them nor have to send them. openapi-typescript
// would make them required unless run with --default-non-nullable=false.
// ErrorModel.type (default "about:blank") is such a field.
export const problemWithoutType: components["schemas"]["ErrorModel"] = {
  title: "Not Found",
  status: 404,
};
