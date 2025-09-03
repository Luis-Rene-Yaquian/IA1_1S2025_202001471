% ======= MediLogic KB (auto-generado) =======
% NO editar a mano; use /admin/kb

sintoma(diarrea).
sintoma(disnea).
sintoma(dolor_garganta).
sintoma(dolor_pecho).
sintoma(fatiga).
sintoma(fiebre).
sintoma(tos).

enfermedad(faringitis, "faringitis", respiratorio, bacteriano).
enfermedad(gripe, "gripe", respiratorio, viral).
enf_sintoma(faringitis, dolor_garganta).
enf_sintoma(faringitis, fiebre).
enf_sintoma(gripe, fiebre).
enf_sintoma(gripe, tos).
enf_sintoma(gripe, fatiga).

medicamento(amoxicilina).
medicamento(ibuprofeno).
medicamento(paracetamol).
trata(amoxicilina, faringitis).
trata(ibuprofeno, gripe).
trata(paracetamol, gripe).
contraindicado(amoxicilina, alergia_penicilina).
contraindicado(amoxicilina, ulcera_gastrica).
contraindicado(ibuprofeno, ulcera_gastrica).
contraindicado(paracetamol, alergia_paracetamol).
