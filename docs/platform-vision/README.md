# Vision de la plateforme mondiale de développement

## Résumé

Cette vision propose de faire évoluer le socle Coder en une plateforme mondiale où les développeurs, les équipes, les établissements d’enseignement et les agents d’intelligence artificielle peuvent construire, tester et livrer des logiciels dans des environnements reproductibles.

La plateforme ne serait pas limitée aux écoles et aux universités. L’éducation constituerait un mode spécialisé, au même titre que le développement individuel, les équipes professionnelles, l’open source, la recherche et les entreprises.

> **La plateforme mondiale où les développeurs et les agents IA construisent, testent et livrent des logiciels ensemble.**

## Proposition de valeur

Un développeur se connecte avec GitHub, GitLab ou Bitbucket, sélectionne un dépôt et un template, puis obtient un workspace prêt à l’emploi dans le navigateur. Une IA externe peut ensuite utiliser ce workspace au moyen d’une API sécurisée, dans les limites définies par le développeur.

L’IA peut analyser le code, exécuter les tests, proposer des corrections, créer une branche, préparer un commit et ouvrir une pull request. Le développeur conserve la maîtrise des autorisations, des secrets, du budget, des commandes exécutables et de la fusion finale.

Le parcours central est le suivant :

```text
Dépôt Git
  -> template de développement
  -> workspace isolé
  -> jeton limité pour une IA externe
  -> analyse et exécution contrôlées
  -> tests et vérifications
  -> commit ou pull request
  -> revue humaine
```

## Publics et modes d’utilisation

| Mode | Utilisateurs | Valeur principale |
|---|---|---|
| Learn | Débutants, étudiants et personnes en reconversion | Apprendre dans un environnement guidé |
| Build | Développeurs individuels et freelances | Développer sans installer toute la chaîne locale |
| Team | Startups et équipes professionnelles | Partager des environnements et collaborer |
| Open Source | Mainteneurs et contributeurs | Traiter des issues et accueillir des contributions |
| Enterprise | Grandes organisations | Gouvernance, sécurité, SSO, audit et contrôle des coûts |
| Research | Laboratoires et chercheurs | Environnements reproductibles et données contrôlées |
| Agent | Agents IA et outils tiers | Exécuter des tâches de développement avec des permissions limitées |

## Fonctionnalités principales

### Developer Cloud

Le Developer Cloud est l’espace destiné aux développeurs humains. Il permet de connecter des fournisseurs Git, d’importer un dépôt, de créer un workspace depuis un template, d’utiliser un éditeur et un terminal dans le navigateur, puis de conserver ou détruire l’environnement selon les besoins.

Les intégrations initiales sont GitHub, GitLab et Bitbucket. Elles doivent être conçues derrière une interface commune afin que les fonctions de dépôt, branche, commit, issue, pull request et webhook ne soient pas dupliquées dans toute la plateforme.

### Agent Runtime

L’Agent Runtime est la couche qui permet à une IA externe d’utiliser la plateforme. Il doit fournir une API REST, un CLI et un serveur MCP. Les outils tiers peuvent ainsi créer un workspace, consulter son état, exécuter une commande autorisée, lancer les tests, récupérer les logs et créer une contribution.

Les agents peuvent être spécialisés : tuteur, développeur, testeur, auditeur de sécurité, agent DevOps, agent de documentation ou agent de support. La plateforme doit rester agnostique vis-à-vis du modèle utilisé.

### Real Issue Lab

Real Issue Lab transforme une issue réelle en mission de développement. Une tâche GitHub, GitLab, un ticket interne ou un exercice peut être associé à un dépôt, une branche de départ, un template, des tests et des critères de réussite.

Le parcours recommandé est :

1. sélectionner une issue ou une mission ;
2. créer un workspace isolé depuis le dépôt ;
3. analyser le problème ;
4. laisser un développeur ou un agent effectuer le travail ;
5. exécuter les tests et les contrôles de qualité ;
6. produire un commit ou une pull request ;
7. effectuer une revue humaine avant fusion.

### Course Studio

Course Studio est le mode éducatif. Un enseignant peut créer un cours, importer un dépôt, distribuer un environnement identique à chaque étudiant, publier des exercices, configurer des tests et suivre la progression de la classe.

L’agent peut fonctionner en mode tuteur, avec des indices progressifs et des explications, sans révéler automatiquement la solution. Le système peut conserver l’historique Git, les tests exécutés, les demandes faites à l’IA et les étapes du travail.

### Portfolio développeur

Chaque utilisateur peut disposer d’un portfolio basé sur du travail vérifiable : projets, commits, pull requests, tests écrits, corrections de bugs, documentation, revues de code et compétences démontrées.

Le portfolio doit valoriser le processus autant que le résultat. Il peut être utilisé par un étudiant, un candidat, un freelance ou un contributeur open source.

### Catalogue et marketplace

Un catalogue institutionnel ou public peut proposer des templates, images, environnements, cours et agents spécialisés. Les éléments doivent être versionnés, documentés, évalués et soumis à des règles de sécurité.

Une marketplace ultérieure peut permettre à des créateurs de publier des templates et des agents premium. La plateforme peut prélever une commission ou facturer l’hébergement et l’exécution.

## API et jetons d’accès

Une IA externe ne doit jamais recevoir une clé générale donnant accès à tout le compte. Le système doit utiliser des jetons temporaires, révocables et limités.

Chaque jeton doit pouvoir être restreint par :

| Dimension | Exemple de restriction |
|---|---|
| Dépôt | Un seul dépôt privé ou public |
| Branche | Une branche de travail, sans accès direct à `main` |
| Workspace | Un environnement précis |
| Actions | Lecture, écriture, tests, pull request |
| Commandes | Allowlist de commandes autorisées |
| Réseau | Domaines ou destinations autorisés |
| Durée | Expiration après une durée courte |
| Ressources | CPU, mémoire, GPU et durée maximale |
| Budget | Montant maximal d’utilisation |
| Secrets | Aucun accès par défaut |

Le jeton doit être affiché une seule fois, stocké de manière sécurisée, révocable immédiatement et associé à un journal d’utilisation.

Exemple d’API conceptuelle :

```text
POST /api/v1/workspaces
POST /api/v1/workspaces/{id}/commands
GET  /api/v1/workspaces/{id}/logs
POST /api/v1/workspaces/{id}/tests
POST /api/v1/workspaces/{id}/pull-requests
POST /api/v1/tokens
DELETE /api/v1/tokens/{id}
```

## Sécurité et confiance

Un agent externe doit être considéré comme non fiable par défaut. La plateforme doit appliquer le principe du moindre privilège, isoler les workspaces, limiter le réseau, protéger les secrets et journaliser les actions importantes.

Le flux par défaut doit être :

```text
branche isolée
  -> workspace isolé
  -> commandes contrôlées
  -> tests
  -> revue humaine
  -> pull request
  -> fusion explicite
```

Les fonctions d’administration, l’accès aux autres dépôts, les secrets de production, les clés cloud et la fusion automatique dans la branche principale doivent être refusés par défaut.

Les établissements et entreprises doivent pouvoir déployer la plateforme sur leur infrastructure, appliquer leurs propres modèles de sécurité, conserver leurs journaux et choisir les fournisseurs de modèles autorisés.

## Ressources et modèle économique

La plateforme peut gagner de l’argent en facturant les ressources réellement consommées et les services à valeur ajoutée.

| Ressource ou service | Mode de facturation possible |
|---|---|
| CPU et mémoire | Durée et taille du workspace |
| GPU | Temps d’utilisation |
| Stockage | Volume et durée de conservation |
| Trafic réseau | Volume sortant selon l’offre |
| Workspaces persistants | Abonnement ou coût horaire |
| Workspaces temporaires | Facturation à la minute |
| Agents hébergés | Exécution, orchestration et supervision |
| Logs et artefacts | Volume et durée de conservation |
| Templates premium | Abonnement, licence ou commission |
| Support et déploiement privé | Offre entreprise |

Les coûts doivent être séparés en trois catégories : infrastructure, services de la plateforme et services de fournisseurs externes. Si l’utilisateur apporte sa propre clé de modèle, la plateforme facture principalement l’infrastructure et l’orchestration. Si elle fournit le modèle, le coût de celui-ci doit apparaître séparément.

Les offres peuvent être structurées ainsi :

| Offre | Positionnement |
|---|---|
| Free | Ressources limitées et templates publics |
| Developer | Dépôts privés, workspaces persistants et davantage de ressources |
| Team | Collaboration, quotas partagés, audit et facturation centralisée |
| Enterprise | SSO, politiques avancées, réseau privé, support et self-hosted |
| Platform/API | Facturation des appels et ressources consommées par des outils tiers |

## Architecture par couches

Le socle doit être organisé en quatre niveaux :

| Niveau | Responsabilité |
|---|---|
| Infrastructure | Workspaces, réseau, images, stockage, quotas et cycle de vie |
| Plateforme | Identité, organisations, dépôts, agents, API, permissions et audit |
| Expériences | Developer Cloud, Real Issue Lab, Course Studio, portfolio et console équipe |
| Écosystème | Catalogue, marketplace, partenaires, agents et intégrations externes |

Le code Coder existant sert principalement de base pour les deux premiers niveaux. Les fonctions pédagogiques, marketplace et portfolio doivent être développées comme des modules distincts, avec leurs propres permissions, API, tests et interfaces.

## MVP recommandé

Le premier produit doit valider un parcours universel plutôt que tenter de construire immédiatement toute la plateforme.

> **Un développeur se connecte avec GitHub, choisit un dépôt et un template, crée un workspace, génère un jeton limité, autorise une IA externe à utiliser ce workspace, puis obtient des tests et une pull request.**

| Fonction | Priorité |
|---|---:|
| Connexion GitHub | Très haute |
| Sélection du dépôt et de la branche | Très haute |
| Création d’un workspace depuis un template | Très haute |
| Jetons limités et révocables | Très haute |
| API workspace | Très haute |
| Exécution de commandes contrôlées | Très haute |
| Logs et audit | Très haute |
| Limites de temps et de ressources | Très haute |
| Tests automatisés | Haute |
| Commit et pull request | Haute |
| GitLab et Bitbucket | Haute |
| Facturation par usage | Haute |
| MCP officiel | Haute |
| Marketplace d’agents | Phase suivante |
| Cours et portfolios | Phase suivante |
| GPU et recherche | Phase suivante |

## Feuille de route

### Phase 1 : noyau développeur

Mettre en place les comptes, la connexion GitHub, le premier template, la création du workspace et la consultation de son état.

### Phase 2 : Agent Runtime

Ajouter l’API, les jetons limités, l’exécution contrôlée, les logs, les tests et un premier SDK ou CLI.

### Phase 3 : Real Issue Lab

Connecter une issue à un dépôt et permettre de produire une branche, des changements, des tests et une pull request.

### Phase 4 : GitLab, Bitbucket et intégrations

Ajouter les autres fournisseurs Git derrière l’interface commune, puis les webhooks, les systèmes de tickets et les pipelines CI/CD.

### Phase 5 : facturation et catalogue

Mesurer les ressources, gérer les quotas, afficher les coûts, proposer les plans et publier les premiers templates validés.

### Phase 6 : éducation et entreprise

Ajouter Course Studio, classes, évaluations, portfolios, SSO, politiques institutionnelles, audit avancé et déploiements privés.

### Phase 7 : écosystème

Ouvrir la marketplace d’agents et de templates, proposer des agents spécialisés, des intégrations partenaires et des environnements de recherche ou GPU.

## Principes de conception

La plateforme doit respecter les principes suivants :

1. **Ouverte aux IA externes**, mais avec des permissions strictes.
2. **Indépendante des fournisseurs Git et des modèles IA.**
3. **Centrée sur des workspaces reproductibles et jetables.**
4. **Transparente sur les coûts et l’utilisation des ressources.**
5. **Sécurisée par défaut, avec validation humaine pour les actions sensibles.**
6. **Compatible cloud, self-hosted et environnements privés.**
7. **Construite par modules afin de pouvoir évoluer sans fragiliser le socle.**
8. **Maintenue par branches et pull requests, sans modification directe de `main`.**

## Résumé final

La vision est de créer une **infrastructure mondiale de production logicielle**. GitHub, GitLab et Bitbucket apportent les dépôts. Les templates apportent les environnements. Les workspaces apportent l’exécution. Les agents IA apportent l’automatisation. L’API permet à des outils externes de tout orchestrer. La sécurité, l’audit et la facturation rendent le service exploitable à grande échelle.

Les écoles et universités ne sont qu’un des marchés. Le même noyau peut servir les développeurs individuels, les équipes professionnelles, les startups, les projets open source, les grandes entreprises et les laboratoires de recherche.
