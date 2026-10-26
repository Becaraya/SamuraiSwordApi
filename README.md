# Samurai Sword API

API REST et WebSocket pour le jeu de cartes multijoueur Samurai Sword, développée en Go avec le framework Gin.

## 🚀 Technologies

- **Go 1.24.5** - Langage principal

## ⚙️ Installation et Configuration

### Prérequis

- Go 1.23.5+
- Docker & Docker Compose (optionnel)

### Configuration de l'environnement

1. **Copier le fichier d'exemple**
```bash
cp .env.example .env
```

2. **Modifier les variables d'environnement**

### Variables d'environnement

| Variable | Description | Défaut |
|----------|-------------|---------|
| `APP_ENV` | Environnement (development/production) | `development` |
| `FRONTEND_URL` | URL du frontend pour CORS | `http://localhost:8000` |
| `SERVER_ADDRESS` | Adresse d'écoute du serveur | `:8080` |
| `PORT` | Port d'écoute | `8080` |
| `CONTEXT_TIMEOUT` | Timeout des requêtes (secondes) | `2` |
| `ACCESS_TOKEN_EXPIRY_HOUR` | Durée de vie access token (heures) | `2` |
| `REFRESH_TOKEN_EXPIRY_HOUR` | Durée de vie refresh token (heures) | `168` |
| `ACCESS_TOKEN_SECRET` | Clé secrète pour les access tokens | **À définir** |
| `REFRESH_TOKEN_SECRET` | Clé secrète pour les refresh tokens | **À définir** |

## 🐳 Démarrage avec Docker

```bash
# Construction et démarrage
docker compose up --build

# Compatible avec Docker Watch
docker compose up --build --watch

# Voir les logs
docker compose logs -f

# Arrêt
docker compose down
```

## 💻 Démarrage en développement sans Docker

```bash
# Installation des dépendances
go mod tidy

# Démarrage du serveur
go run cmd/main.go
```

L'API sera accessible sur `http://localhost:8080`

## 🔧 Configuration CORS

L'API est configurée pour accepter les requêtes cross-origin :

```go
gin.Use(cors.New(cors.Config{
    AllowOrigins:     []string{env.FrontendUrl},
    AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
    AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
    AllowCredentials: true,
    MaxAge:           12 * time.Hour,
}))
```

### Logs utiles

```bash
# Logs Docker
docker-compose logs -f app

# Logs en développement
go run cmd/main.go
```

## 🔒 Sécurité

### Clés secrètes
```bash
openssl rand -base64 32

```

## 🚀 Déploiement en Production

### Prérequis production
- Serveur avec Docker
- Base de données (si ajoutée)
- Certificats SSL/TLS
- Domaine configuré

### Variables d'environnement production
```env
APP_ENV=production
FRONTEND_URL=https://votre-domaine.com
SERVER_ADDRESS=:443
ACCESS_TOKEN_SECRET=cle_super_secrete_longue
REFRESH_TOKEN_SECRET=autre_cle_super_secrete_longue
```

## 📚 Dépendances

### Principales dépendances
```go
require (
    github.com/gin-gonic/gin v1.10.0
    github.com/gin-contrib/cors v1.7.3
    github.com/golang-jwt/jwt/v5 v5.2.1
    github.com/gorilla/websocket v1.5.3
    github.com/spf13/viper v1.19.0
)
```

## 🤝 Contribution

1. Fork le projet
2. Créer une branche feature (`git checkout -b feature/nouvelle-fonctionnalite`)
3. Commit les changements (`git commit -m 'Ajout nouvelle fonctionnalité'`)
4. Push sur la branche (`git push origin feature/nouvelle-fonctionnalite`)
5. Ouvrir une Pull Request
