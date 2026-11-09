---
name: Le journal avant tout
level: 1
state: progress
links: [raft]
tags: [livre, bases-de-donnees, durabilite]
source:
  title: Le journal avant tout
  author: Road to Knowledge
  year: 2026
  pages: 7
  file: library/journal-6e5505f8.pdf
  sha256: 6e5505f88c11479b86e89c5eddb4f414338e4452cd464db3b846b228ac4b51fe
---
Court guide sur la durabilité : comment une base garantit qu'une transaction validée survit à une panne, du journal d'écriture anticipée jusqu'à la réplication. Utile pour lire la doc de PostgreSQL ou d'etcd sans se perdre.

## À retenir : trois caches avant le disque (p. 2)
Entre la validation et le support physique, une donnée passe par le tampon de la base, le cache de pages du noyau et le cache du disque. Une coupure les vide tous ; répondre « validé » trop tôt revient à mentir au client.

Quelle propriété ACID garantit qu'une transaction validée survit à une panne ? :: La durabilité
Pourquoi écrire les pages de données à chaque validation est-il lent ? :: Cela produit des écritures aléatoires dispersées sur le disque

## À retenir : la règle du WAL (p. 3)
On rend durable la description d'une modification dans un journal en ajout seul avant de toucher la page de données. Les pages modifiées rejoignent le disque plus tard, en lot ; le journal suffit à les reconstruire.

Que signifie WAL ? :: Write-ahead log, journal d'écriture anticipée
Dans un WAL, qu'est-ce qui doit être durable avant qu'une page de données soit écrite ? :: L'enregistrement du journal qui décrit sa modification
À quoi sert le LSN d'un enregistrement de journal ? :: À ordonner toutes les modifications de la base
Combien d'écritures synchrones coûte la validation d'une transaction avec un WAL ? :: Une seule, séquentielle, dans le journal
Comment s'appelle le partage d'une écriture du journal entre plusieurs validations simultanées ? :: La validation groupée (group commit)

## À retenir : fsync est la seule frontière (p. 4)
`write()` ne fait que copier dans le cache du noyau. Il faut `fsync()` sur le fichier, et aussi sur son répertoire après une création ou un renommage. Un `fsync()` en échec peut laisser le noyau oublier les pages : le réessayer ne prouve rien, d'où l'arrêt de PostgreSQL depuis 2018.

Après un write() réussi sous Linux, où sont les octets ? :: Dans le cache de pages du noyau
Pourquoi faire fsync() sur le répertoire après avoir créé un fichier ? :: Pour que l'entrée du fichier dans le répertoire survive à une panne
Pourquoi réessayer un fsync() qui a échoué est-il dangereux ? :: Le noyau peut avoir marqué les pages comme propres, le second appel réussit sans rien écrire
Que fait PostgreSQL depuis 2018 quand fsync() échoue ? :: Il s'arrête et rejoue le journal au redémarrage

## À retenir : checkpoint et rejeu idempotent (p. 5)
Un point de reprise écrit les pages modifiées jusqu'à un LSN et le note ; la reprise rejoue le journal à partir de là. Chaque page porte le LSN de sa dernière modification, donc rejouer deux fois est sans effet.

Que borne un point de reprise (checkpoint) ? :: La durée de la reprise après panne
Pourquoi le rejeu du journal est-il idempotent ? :: Chaque page garde le LSN de sa dernière modification, un enregistrement plus ancien est ignoré
Qu'est-ce qui permet une restauration à un instant donné (PITR) ? :: Une sauvegarde plus les segments de journal archivés rejoués par-dessus

## À retenir : répliquer, c'est expédier le journal (p. 6)
La réplication en flux envoie le journal au réplica qui le rejoue. Le vrai choix est le moment où l'on répond « validé » : après durabilité locale (asynchrone, perte possible) ou après accusé d'un réplica (synchrone, un aller-retour réseau de plus).

Que reçoit et rejoue un réplica en réplication physique en flux ? :: Le journal du primaire
Quel est le risque de la réplication asynchrone ? :: Perdre les dernières transactions validées si le primaire tombe
Quel est le coût de la réplication synchrone ? :: Un aller-retour réseau à chaque validation
Quand une entrée du journal Raft est-elle validée ? :: Quand une majorité de nœuds l'a rendue durable
