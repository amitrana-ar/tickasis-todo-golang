# Tickasis Todo Application – Kubernetes Deployment Review

## 1. Project Overview

This document describes how the **Tickasis Todo Golang** application is deployed on a local Kubernetes cluster using Helm.

| Item | Technology / Configuration |
|---|---|
| Application | Tickasis Todo Golang |
| Kubernetes environment | Kind (Kubernetes in Docker) |
| Kubernetes version | v1.37.0 |
| Deployment management | Helm |
| Namespace | `tickasis` |
| Container registry | Amazon Elastic Container Registry (ECR) |
| Secrets management | AWS Secrets Manager + External Secrets Operator (ESO) |
| Ingress | NGINX Ingress Controller |
| Database | PostgreSQL |
| Future deployment target | Amazon EKS |

The application consists of four main components:

- Frontend
- Authentication service
- Application backend
- PostgreSQL database

## 2. Deployment Architecture

```text
Developer
   |
   v
Build Docker Images
   |
   v
Amazon ECR
   |
   v
Kind Kubernetes Cluster
   |
   +-------------------- Namespace: tickasis --------------------+
   |                                                              |
   |   NGINX Ingress Controller                                   |
   |          |                                                   |
   |          v                                                   |
   |   tickasis-frontend Service                                  |
   |          |                                                   |
   |          v                                                   |
   |     Frontend Pods                                            |
   |                                                              |
   |   tickasis-auth Service  ---> Authentication Pods            |
   |                                                              |
   |   tickasis-app Service   ---> Backend Pods                    |
   |                                  |                           |
   |                                  v                           |
   |                         tickasis-postgres Service            |
   |                                  |                           |
   |                                  v                           |
   |                           PostgreSQL Pod                     |
   |                                  |                           |
   |                                  v                           |
   |                         PersistentVolumeClaim                |
   +--------------------------------------------------------------+

AWS Secrets Manager
        |
        v
External Secrets Operator
        |
        v
Kubernetes Secret: tickasis-app-secrets
        |
        v
Application Pods
```

## 3. Kubernetes Cluster

A Kind cluster named `tickasis-todo` is used for local deployment. It contains three nodes:

| Node | Role |
|---|---|
| `tickasis-todo-control-plane` | Control plane |
| `tickasis-todo-worker` | Worker |
| `tickasis-todo-worker2` | Worker |

All application resources are deployed in the `tickasis` namespace. External Secrets Operator runs separately in the `external-secrets` namespace.

Useful commands:

```bash
kubectl get nodes
kubectl get all -n tickasis
kubectl get pods -n external-secrets
```

## 4. Container Images – Amazon ECR

Private Docker images are stored in Amazon ECR.

| ECR repository | Component |
|---|---|
| `tickasis-todo-frontend` | Frontend |
| `tickasis-todo-auth` | Authentication service |
| `tickasis-todo-app` | Backend |
| `postgres` | PostgreSQL |

**AWS account:** `727646469309`  
**AWS region:** `us-east-1`  
**Registry:** `727646469309.dkr.ecr.us-east-1.amazonaws.com`

Image references used by the deployment:

```text
727646469309.dkr.ecr.us-east-1.amazonaws.com/tickasis-todo-frontend:latest
727646469309.dkr.ecr.us-east-1.amazonaws.com/tickasis-todo-auth:latest
727646469309.dkr.ecr.us-east-1.amazonaws.com/tickasis-todo-app:latest
727646469309.dkr.ecr.us-east-1.amazonaws.com/postgres:16-alpine
```

Kubernetes uses the `ecr-registry-secret` image pull Secret in the `tickasis` namespace to authenticate when pulling private images. ECR authorization tokens are valid for 12 hours and the Secret must be refreshed when required.

Refresh command:

```bash
kubectl create secret docker-registry ecr-registry-secret \
  --namespace tickasis \
  --docker-server=727646469309.dkr.ecr.us-east-1.amazonaws.com \
  --docker-username=AWS \
  --docker-password="$(aws ecr get-login-password --region us-east-1)" \
  --dry-run=client -o yaml | kubectl apply -f -
```

## 5. Helm Chart

Helm is used to package and deploy the application's Kubernetes resources. The chart keeps deployment configuration in `values.yaml` and Kubernetes resource definitions in reusable templates.

Chart location:

```text
helm/tickasis/
```

### Chart directory structure

```text
tickasis-todo-golang/
├── helm/
│   └── tickasis/
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
│           ├── app-deployment.yaml
│           ├── app-svc.yaml
│           ├── auth-deployment.yaml
│           ├── auth-svc.yaml
│           ├── frontend-deployment.yaml
│           ├── frontend-svc.yaml
│           ├── postgres-deployment.yaml
│           ├── postgres-svc.yaml
│           ├── pvc.yaml
│           ├── configmap.yaml
│           ├── external-secret.yaml
│           ├── ingress.yaml
│           └── serviceaccount.yaml
```

The templates define the application Deployments and Services, PostgreSQL storage, application configuration, secret synchronization, Ingress routing, and ServiceAccount configuration.

### Main Helm configuration

`values.yaml` contains configurable values such as:

- Container image repositories, tags, and pull policies
- Deployment replica counts
- Container ports and Service ports
- CPU and memory resource settings
- PostgreSQL persistent storage settings
- Ingress enablement and hostname

Validate and render the chart:

```bash
helm lint ./helm/tickasis
helm template tickasis ./helm/tickasis -n tickasis
```

Install or update the application:

```bash
helm install tickasis ./helm/tickasis -n tickasis
helm upgrade tickasis ./helm/tickasis -n tickasis
```

## 6. Application Deployments and Services

Each application component is managed by a Kubernetes Deployment and exposed internally through a ClusterIP Service.

| Component | Deployment | Service | Service port |
|---|---|---|---:|
| Frontend | `tickasis-frontend` | `tickasis-frontend` | 8080 |
| Authentication | `tickasis-auth` | `tickasis-auth` | 8081 |
| Backend | `tickasis-app` | `tickasis-app` | 8082 |
| PostgreSQL | `tickasis-postgres` | `tickasis-postgres` | 5432 |

The application uses Kubernetes Service DNS for internal communication:

```text
AUTH_SERVICE_URL=http://tickasis-auth:8081
TODO_SERVICE_URL=http://tickasis-app:8082
```

Check Deployments, Pods, and Services:

```bash
kubectl get deployments -n tickasis
kubectl get pods -n tickasis -o wide
kubectl get services -n tickasis
```

## 7. Persistent Storage

PostgreSQL data is stored using a PersistentVolumeClaim.

| Setting | Value |
|---|---|
| PVC | `tickasis-postgres-pvc` |
| Storage request | `10Gi` |
| Access mode | `ReadWriteOnce` |
| StorageClass | `standard` |
| Kind provisioner | `rancher.io/local-path` |

The PostgreSQL container mounts the persistent volume at:

```text
/var/lib/postgresql/data
```

Check storage resources:

```bash
kubectl get pvc -n tickasis
kubectl get pv
kubectl get storageclass
```

For production, database availability and storage should be designed separately from this local Kind setup. Amazon RDS is one option for the EKS environment.

## 8. AWS Secrets Manager and External Secrets Operator

Application database configuration is stored in AWS Secrets Manager.

| Setting | Value |
|---|---|
| AWS secret name | `tickasis-app` |
| AWS region | `us-east-1` |
| External Secrets Operator namespace | `external-secrets` |
| ClusterSecretStore | `aws-secrets-manager` |
| ExternalSecret | `tickasis-app-secret` |
| Kubernetes target Secret | `tickasis-app-secrets` |
| Application namespace | `tickasis` |

The AWS secret contains these keys:

```text
POSTGRES_DB
POSTGRES_USER
POSTGRES_PASSWORD
DATABASE_URL
```

ESO retrieves the `tickasis-app` secret from AWS Secrets Manager and synchronizes its values into the Kubernetes Secret `tickasis-app-secrets`. Application containers consume the required values through Kubernetes environment variables.

For the local Kind environment, ESO uses AWS credentials stored in the Kubernetes Secret `aws-secret-manager-credentials` in the `external-secrets` namespace. A dedicated IAM identity and least-privilege policy provide access to the required Secrets Manager secret.

Check the secret synchronization resources:

```bash
kubectl get clustersecretstore
kubectl get externalsecret -n tickasis
kubectl get secret tickasis-app-secrets -n tickasis
```

Do not store actual passwords, access keys, or database connection strings in this document or in source control.

## 9. NGINX Ingress Controller

The NGINX Ingress Controller is installed in the `ingress-nginx` namespace. The application Ingress resource is managed through the Helm chart in the `tickasis` namespace.

The Ingress routes requests for the hostname `tickasis.local` to the frontend Service:

| Setting | Value |
|---|---|
| Ingress resource | `tickasis-ingress` |
| Ingress class | `nginx` |
| Hostname | `tickasis.local` |
| Path | `/` |
| Backend Service | `tickasis-frontend` |
| Backend port | `8080` |

### Local access from Windows through WSL2

The Kind NGINX controller Service is configured as `LoadBalancer`. Kind does not automatically provision an external load balancer, so its external IP remains pending. Local access is provided using `kubectl port-forward`.

Run in the WSL Ubuntu terminal and keep the command running:

```bash
kubectl port-forward -n ingress-nginx \
  svc/ingress-nginx-controller 8088:80 \
  --address 0.0.0.0
```

The Windows hosts file must resolve `tickasis.local` to the reachable WSL address. Open the application at:

```text
http://tickasis.local:8088
```

The configured hostname is important because the Ingress rule routes requests based on the HTTP host header.

Check the Ingress resource and controller:

```bash
kubectl get ingress -n tickasis
kubectl get services -n ingress-nginx
kubectl get pods -n ingress-nginx
```

The controller also has an `ingress-nginx-controller-admission` Service. This internal Service is used by the Kubernetes admission webhook to validate Ingress resources; it does not route normal application traffic.

The local Ingress setup currently uses HTTP. TLS is not configured.

## 10. Deployment Procedure

The deployment can be reproduced at a high level using this sequence:

1. Create or start the Kind cluster.
2. Create the `tickasis` namespace.
3. Build the application Docker images and push them to the ECR repositories.
4. Create or refresh the `ecr-registry-secret` in the `tickasis` namespace.
5. Install External Secrets Operator in the `external-secrets` namespace.
6. Configure AWS IAM access for ESO.
7. Create the AWS credentials Secret used by ESO in the local Kind environment.
8. Apply the `ClusterSecretStore` configuration.
9. Deploy the application's `ExternalSecret` through the Helm chart.
10. Configure the Helm chart values and Kubernetes templates.
11. Install or upgrade the application using Helm.
12. Install/configure the NGINX Ingress Controller and deploy the Ingress resource.
13. Start the Ingress port-forward and access the application through `tickasis.local`.

Main Helm deployment commands:

```bash
helm lint ./helm/tickasis
helm upgrade --install tickasis ./helm/tickasis -n tickasis
```

Verify the deployed resources:

```bash
kubectl get pods -n tickasis
kubectl get deployments -n tickasis
kubectl get services -n tickasis
kubectl get pvc -n tickasis
kubectl get externalsecret -n tickasis
kubectl get ingress -n tickasis
```

## 11. Current Deployment Summary

The application components were observed running in the `tickasis` namespace, with Pods reporting `Running` and containers ready. The Ingress host-based route to the frontend was also verified: a request to `http://tickasis.local:8088` returned an HTTP `303 See Other` redirect to `/signin`.

This confirms that the local Ingress route reaches the frontend. Full application workflows, including signup, signin, and database operations, should be validated separately.

## 12. Production Deployment Considerations

The current environment is a local Kind deployment. For a future Amazon EKS deployment, review these areas:

| Area | Current local setup | EKS consideration |
|---|---|---|
| Kubernetes | Kind | Amazon EKS |
| Image registry | Amazon ECR | Continue using ECR |
| Image pull authentication | Kubernetes image pull Secret | EKS node IAM permissions |
| AWS secrets access | IAM user credentials for local ESO | IRSA or EKS Pod Identity |
| Secrets synchronization | External Secrets Operator | Continue with ESO |
| Database | PostgreSQL in Kubernetes | Consider Amazon RDS |
| Persistent storage | Kind local-path | EBS CSI or managed database storage |
| Ingress | NGINX Ingress Controller | AWS Load Balancer Controller or an appropriately managed Ingress solution |
| DNS and TLS | Local hostname and HTTP | DNS and TLS configuration for the chosen ingress architecture |
| Image tags | `latest` in current image references | Immutable commit SHA or release tags |

For EKS, use workload IAM roles rather than long-lived AWS access keys stored in Kubernetes Secrets. Production database replication, backup, monitoring, logging, network access, and TLS should also be designed for the target environment.

## 13. Useful Documentation

- Kubernetes: https://kubernetes.io/docs/
- Helm: https://helm.sh/docs/
- Kind: https://kind.sigs.k8s.io/
- Amazon ECR: https://docs.aws.amazon.com/AmazonECR/latest/userguide/what-is-ecr.html
- Amazon EKS: https://docs.aws.amazon.com/eks/latest/userguide/what-is-eks.html
- AWS Secrets Manager: https://docs.aws.amazon.com/secretsmanager/latest/userguide/intro.html
- External Secrets Operator: https://external-secrets.io/
