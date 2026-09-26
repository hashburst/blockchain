HVM Network — ripresa API 1.1.1
Eseguire sul Mac: shasum -a 256 -c SHA256SUMS && python3 rollout.py
Verifica v1 aggiornato tramite binario effettivo, journal originale, pin, avanzamento e commitment; non lo riavvia. Aggiorna gli altri nodi uno alla volta. Attesa replay massima 60 minuti per nodo, sessione SSH dedicata. In errore conserva dati e journal: nessun rollback automatico. Infine confronta i commitment alla stessa altezza finalizzata. Observer e canary non inclusi. Binario invariato rispetto a 1.1.0.
