import { IdleWatcherConfig } from "./providers/idlewatcher";
import { Route } from "./providers/routes";
export type DockerRoutes = {
  [key: string]: Route & IdleWatcherConfig;
};
