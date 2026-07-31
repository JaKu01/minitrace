import * as docker from "@pulumi/docker";
import * as k8s from "@pulumi/kubernetes";
import * as pulumi from "@pulumi/pulumi";

const appName = "minitrace";
const config = new pulumi.Config();
const namespace = config.get("namespace") ?? "home-server";
const hostname = config.get("hostname") ?? "traces.kurth.dev";
const imageName = config.get("imageName") ?? `registry.kurth.dev/${appName}:latest`;
const storageSize = config.get("storageSize") ?? "2Gi";
const requestedRetentionHours = config.getNumber("retentionHours") ?? 24;
const retentionHours = Math.max(1, Math.min(24, requestedRetentionHours));

const image = new docker.Image(`${appName}-image`, {
  imageName,
  build: {
    context: "../",
    dockerfile: "../Dockerfile",
  },
});

const data = new k8s.core.v1.PersistentVolumeClaim(`${appName}-data`, {
  metadata: {
    name: `${appName}-data`,
    namespace,
    labels: { app: appName },
  },
  spec: {
    accessModes: ["ReadWriteOnce"],
    resources: {
      requests: { storage: storageSize },
    },
  },
});

const deployment = new k8s.apps.v1.Deployment(appName, {
  metadata: {
    name: appName,
    namespace,
    labels: { app: appName },
  },
  spec: {
    replicas: 1,
    strategy: { type: "Recreate" },
    selector: { matchLabels: { app: appName } },
    template: {
      metadata: { labels: { app: appName } },
      spec: {
        securityContext: {
          fsGroup: 10001,
          fsGroupChangePolicy: "OnRootMismatch",
        },
        containers: [{
          name: appName,
          image: image.repoDigest,
          imagePullPolicy: "Always",
          args: [
            "--database", "/data/minitrace.db",
            "--retention", `${retentionHours}h`,
          ],
          ports: [{ name: "http", containerPort: 8080 }],
          volumeMounts: [{ name: "data", mountPath: "/data" }],
          readinessProbe: {
            httpGet: { path: "/api/v1/health", port: "http" },
            initialDelaySeconds: 2,
            periodSeconds: 10,
          },
          livenessProbe: {
            httpGet: { path: "/api/v1/health", port: "http" },
            initialDelaySeconds: 10,
            periodSeconds: 20,
          },
          securityContext: {
            allowPrivilegeEscalation: false,
            readOnlyRootFilesystem: true,
            runAsNonRoot: true,
            runAsUser: 10001,
            runAsGroup: 10001,
            capabilities: { drop: ["ALL"] },
          },
          resources: {
            requests: { cpu: "20m", memory: "48Mi" },
            limits: { cpu: "500m", memory: "256Mi" },
          },
        }],
        volumes: [{
          name: "data",
          persistentVolumeClaim: { claimName: data.metadata.name },
        }],
      },
    },
  },
});

const service = new k8s.core.v1.Service(appName, {
  metadata: { name: appName, namespace },
  spec: {
    type: "ClusterIP",
    selector: { app: appName },
    ports: [{ name: "http", port: 8080, targetPort: "http" }],
  },
});

const ingressRoute = new k8s.apiextensions.CustomResource(`${appName}-ingressroute`, {
  apiVersion: "traefik.io/v1alpha1",
  kind: "IngressRoute",
  metadata: { name: `${appName}-ingressroute`, namespace },
  spec: {
    entryPoints: ["websecure"],
    routes: [{
      match: `Host(\`${hostname}\`)`,
      kind: "Rule",
      middlewares: [{ name: "homeserver-auth", namespace }],
      services: [{ name: service.metadata.name, port: 8080 }],
    }],
  },
}, { dependsOn: [deployment, service] });

export const dashboardUrl = pulumi.interpolate`https://${hostname}`;
export const serviceEndpoint = pulumi.interpolate`http://${service.metadata.name}.${namespace}.svc.cluster.local:8080`;
export const persistentVolumeClaim = data.metadata.name;
export const deploymentName = deployment.metadata.name;
export const ingressRouteName = ingressRoute.metadata.name;
